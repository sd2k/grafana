package ualert

import (
	"fmt"
	"os"

	"github.com/grafana/grafana/pkg/services/sqlstore/migrator"
	"xorm.io/xorm"
)

const GENERAL_FOLDER = "General Alerting"
const DASHBOARD_FOLDER = "Migrated %s"

// FOLDER_CREATED_BY us used to track folders created by this migration
// during alert migration cleanup.
const FOLDER_CREATED_BY = -8

var migTitle = "move dashboard alerts to unified alerting"

var rmMigTitle = "remove unified alerting data"

type MigrationError struct {
	AlertId int64
	Err     error
}

func (e MigrationError) Error() string {
	return fmt.Sprintf("failed to migrate alert %d: %s", e.AlertId, e.Err.Error())
}

func (e *MigrationError) Unwrap() error { return e.Err }

func AddMigration(mg *migrator.Migrator) {
	if os.Getenv("UALERT_MIG") == "iDidBackup" {
		// TODO: unified alerting DB needs to be extacted into ../migrations.go
		// so it runs and creates the tables before this migration runs.
		logs, err := mg.GetMigrationLog()
		if err != nil {
			mg.Logger.Crit("alert migration failure: could not get migration log", "error", err)
			os.Exit(1)
		}

		_, migrationRun := logs[migTitle]

		ngEnabled := mg.Cfg.IsNgAlertEnabled()

		switch {
		case ngEnabled && !migrationRun:
			// clear the entry of the migration that
			err = mg.ClearMigrationEntry(rmMigTitle)
			if err != nil {
				mg.Logger.Error("alert migration error: could not clear alert migration for removing data", "error", err)
			}
			mg.AddMigration(migTitle, &migration{})
		case !ngEnabled && migrationRun:
			err = mg.ClearMigrationEntry(migTitle)
			if err != nil {
				mg.Logger.Error("alert migration error: could not clear alert migration", "error", err)
			}
			mg.AddMigration(rmMigTitle, &rmMigration{})
		}
	}
}

type migration struct {
	migrator.MigrationBase
	// session and mg are attached for convenience.
	sess *xorm.Session
	mg   *migrator.Migrator
}

func (m *migration) SQL(dialect migrator.Dialect) string {
	return "code migration"
}

func (m *migration) Exec(sess *xorm.Session, mg *migrator.Migrator) error {
	m.sess = sess
	m.mg = mg

	dashAlerts, err := m.slurpDashAlerts()
	if err != nil {
		return err
	}

	// [orgID, dataSourceId] -> UID
	dsIDMap, err := m.slurpDSIDs()
	if err != nil {
		return err
	}

	// [orgID, dashboardId] -> dashUID
	dashIDMap, err := m.slurpDashUIDs()
	if err != nil {
		return err
	}

	for _, da := range dashAlerts {
		newCond, err := transConditions(*da.ParsedSettings, da.OrgId, dsIDMap)
		if err != nil {
			return err
		}

		da.DashboardUID = dashIDMap[[2]int64{da.OrgId, da.DashboardId}]

		// get dashboard
		dash := dashboard{}
		exists, err := m.sess.Where("org_id=? AND uid=?", da.OrgId, da.DashboardUID).Get(&dash)
		if err != nil {
			return MigrationError{
				Err:     fmt.Errorf("failed to get dashboard %s under organisation %d: %w", da.DashboardUID, da.OrgId, err),
				AlertId: da.Id,
			}
		}
		if !exists {
			return MigrationError{
				Err:     fmt.Errorf("dashboard with UID %v under organisation %d not found: %w", da.DashboardUID, da.OrgId, err),
				AlertId: da.Id,
			}
		}

		// get folder if exists
		folder := dashboard{}
		if dash.FolderId > 0 {
			exists, err := m.sess.Where("id=?", dash.FolderId).Get(&folder)
			if err != nil {
				return MigrationError{
					Err:     fmt.Errorf("failed to get folder %d: %w", dash.FolderId, err),
					AlertId: da.Id,
				}
			}
			if !exists {
				return MigrationError{
					Err:     fmt.Errorf("folder with id %v not found", dash.FolderId),
					AlertId: da.Id,
				}
			}
			if !folder.IsFolder {
				return MigrationError{
					Err:     fmt.Errorf("id %v is a dashboard not a folder", dash.FolderId),
					AlertId: da.Id,
				}
			}
		}

		switch {
		case dash.HasAcl:
			// create folder and assign the permissions of the dashboard (included default and inherited)
			ptr, err := m.createFolder(dash.OrgId, fmt.Sprintf(DASHBOARD_FOLDER, getMigrationString(da)))
			if err != nil {
				return MigrationError{
					Err:     fmt.Errorf("failed to create folder: %w", err),
					AlertId: da.Id,
				}
			}
			folder = *ptr
			permissions, err := m.getACL(dash.OrgId, dash.Id)
			if err != nil {
				return MigrationError{
					Err:     fmt.Errorf("failed to get dashboard %d under organisation %d permissions: %w", dash.Id, dash.OrgId, err),
					AlertId: da.Id,
				}
			}
			err = m.setACL(folder.OrgId, folder.Id, permissions)
			if err != nil {
				return MigrationError{
					Err:     fmt.Errorf("failed to set folder %d under organisation %d permissions: %w", folder.Id, folder.OrgId, err),
					AlertId: da.Id,
				}
			}
		case dash.FolderId > 0:
			// link the new rule to the existing folder
		default:
			// get or create general folder
			ptr, err := m.getOrCreateGeneralFolder(dash.OrgId)
			if err != nil {
				return MigrationError{
					Err:     fmt.Errorf("failed to get or create general folder under organisation %d: %w", dash.OrgId, err),
					AlertId: da.Id,
				}
			}
			// No need to assign default permissions to general folder
			// because they are included to the query result if it's a folder with no permissions
			// https://github.com/grafana/grafana/blob/076e2ce06a6ecf15804423fcc8dca1b620a321e5/pkg/services/sqlstore/dashboard_acl.go#L109
			folder = *ptr
		}

		if folder.Uid == "" {
			return MigrationError{
				Err:     fmt.Errorf("empty folder identifier"),
				AlertId: da.Id,
			}
		}
		rule, err := m.makeAlertRule(*newCond, da, folder.Uid)
		if err != nil {
			return err
		}

		_, err = m.sess.Insert(rule)
		if err != nil {
			// TODO better error handling, if constraint
			rule.Title += fmt.Sprintf(" %v", rule.Uid)
			rule.RuleGroup += fmt.Sprintf(" %v", rule.Uid)

			_, err = m.sess.Insert(rule)
			if err != nil {
				return err
			}
		}

		// create entry in alert_rule_version
		_, err = m.sess.Insert(rule.makeVersion())
		if err != nil {
			return err
		}
	}

	return nil
}

type rmMigration struct {
	migrator.MigrationBase
}

func (m *rmMigration) SQL(dialect migrator.Dialect) string {
	return "code migration"
}

func (m *rmMigration) Exec(sess *xorm.Session, mg *migrator.Migrator) error {
	_, err := sess.Exec("delete from alert_rule")
	if err != nil {
		return err
	}

	_, err = sess.Exec("delete from alert_rule_version")
	if err != nil {
		return err
	}

	_, err = sess.Exec("delete from dashboard_acl where dashboard_id IN (select id from dashboard where created_by = ?)", FOLDER_CREATED_BY)
	if err != nil {
		return err
	}

	_, err = sess.Exec("delete from dashboard where created_by = ?", FOLDER_CREATED_BY)
	if err != nil {
		return err
	}

	_, err = sess.Exec("delete from alert_configuration")
	if err != nil {
		return err
	}

	_, err = sess.Exec("delete from alert_instance")
	if err != nil {
		return err
	}

	return nil
}
