package alerting

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/bus"
	"github.com/grafana/grafana/pkg/components/simplejson"
	"github.com/grafana/grafana/pkg/models"
	apimodels "github.com/grafana/grafana/pkg/services/ngalert/api/tooling/definitions"
	ngmodels "github.com/grafana/grafana/pkg/services/ngalert/models"
	"github.com/grafana/grafana/pkg/services/ngalert/notifier/channels"
	"github.com/grafana/grafana/pkg/tests/testinfra"
)

// alertmanagerConfig has the config for all the notification channels
// that we want to test. It is recommended to use different URL for each
// channel and have 1 route per channel.
const alertmanagerConfig = `
{
  "alertmanager_config": {
    "route": {
      "receiver": "slack_recv1",
      "group_wait": "0s",
      "group_by": [
        "alertname"
      ],
      "routes": [
        {
          "receiver": "email_recv",
          "group_wait": "0s",
          "group_by": [
            "alertname"
          ],
          "matchers": [
            "alertname=\"EmailAlert\""
          ]
        },
        {
          "receiver": "slack_recv1",
          "group_wait": "0s",
          "group_by": [
            "alertname"
          ],
          "matchers": [
            "alertname=\"SlackAlert1\""
          ]
        },
        {
          "receiver": "slack_recv2",
          "group_wait": "0s",
          "group_by": [
            "alertname"
          ],
          "matchers": [
            "alertname=\"SlackAlert2\""
          ]
        },
        {
          "receiver": "pagerduty_recv",
          "group_wait": "0s",
          "group_by": [
            "alertname"
          ],
          "matchers": [
            "alertname=\"PagerdutyAlert\""
          ]
        },
        {
          "receiver": "dingding_recv",
          "group_wait": "0s",
          "group_by": [
            "alertname"
          ],
          "matchers": [
            "alertname=\"DingDingAlert\""
          ]
        },
        {
          "receiver": "teams_recv",
          "group_wait": "0s",
          "group_by": [
            "alertname"
          ],
          "matchers": [
            "alertname=\"TeamsAlert\""
          ]
        },
        {
          "receiver": "webhook_recv",
          "group_wait": "0s",
          "group_by": [
            "alertname"
          ],
          "matchers": [
            "alertname=\"WebhookAlert\""
          ]
        },
        {
          "receiver": "telegram_recv",
          "group_wait": "0s",
          "group_by": [
            "alertname"
          ],
          "matchers": [
            "alertname=\"TelegramAlert\""
          ]
        }
      ]
    },
    "receivers": [
      {
        "name": "email_recv",
        "grafana_managed_receiver_configs": [
          {
            "name": "email_test",
            "type": "email",
            "settings": {
              "addresses": "test@email.com",
              "singleEmail": true
            }
          }
        ]
      },
      {
        "name": "dingding_recv",
        "grafana_managed_receiver_configs": [
          {
            "name": "dingding_test",
            "type": "dingding",
            "settings": {
              "url": "http://CHANNEL_ADDR/dingding_recv/dingding_test"
            }
          }
        ]
      },
      {
        "name": "teams_recv",
        "grafana_managed_receiver_configs": [
          {
            "name": "teams_test",
            "type": "teams",
            "settings": {
              "url": "http://CHANNEL_ADDR/teams_recv/teams_test"
            }
          }
        ]
      },
      {
        "name": "webhook_recv",
        "grafana_managed_receiver_configs": [
          {
            "name": "webhook_test",
            "type": "webhook",
            "settings": {
              "url": "http://CHANNEL_ADDR/webhook_recv/webhook_test",
              "username": "my_username",
              "httpMethod": "POST",
              "maxAlerts": "5"
            },
            "secureSettings": {
              "password": "mysecretpassword"
            }
          }
        ]
      },
      {
        "name": "telegram_recv",
        "grafana_managed_receiver_configs": [
          {
            "name": "telegram_test",
            "type": "telegram",
            "settings": {
              "chatid": "telegram_chat_id"
            },
            "secureSettings": {
              "bottoken": "6sh027hs034h"
            }
          }
        ]
      },
      {
        "name": "slack_recv1",
        "grafana_managed_receiver_configs": [
          {
            "name": "slack_test_without_token",
            "type": "slack",
            "settings": {
              "recipient": "#test-channel",
              "mentionChannel": "here",
              "mentionUsers": "user1, user2",
              "mentionGroups": "group1, group2",
              "username": "Integration Test",
              "icon_emoji": "🚀",
              "icon_url": "https://awesomeemoji.com/rocket",
              "text": "Integration Test {{ template \"slack.default.text\" . }}",
              "title": "Integration Test {{ template \"slack.default.title\" . }}",
              "fallback": "Integration Test {{ template \"slack.default.title\" . }}"
            },
            "secureSettings": {
              "url": "http://CHANNEL_ADDR/slack_recv1/slack_test_without_token"
            }
          }
        ]
      },
      {
        "name": "slack_recv2",
        "grafana_managed_receiver_configs": [
          {
            "name": "slack_test_with_token",
            "type": "slack",
            "settings": {
              "recipient": "#test-channel",
              "mentionUsers": "user1, user2",
              "username": "Integration Test"
            },
            "secureSettings": {
              "token": "myfullysecrettoken"
            }
          }
        ]
      },
      {
        "name": "pagerduty_recv",
        "grafana_managed_receiver_configs": [
          {
            "name": "pagerduty_test",
            "type": "pagerduty",
            "settings": {
              "severity": "warning",
              "class": "testclass",
              "component": "Integration Test",
              "group": "testgroup",
              "summary": "Integration Test {{ template \"pagerduty.default.description\" . }}"
            },
            "secureSettings": {
              "integrationKey": "pagerduty_recv/pagerduty_test"
            }
          }
        ]
      }
    ]
  }
}
`

var expAlertmanagerConfigFromAPI = `
{
  "template_files": null,
  "alertmanager_config": {
    "route": {
      "receiver": "slack_recv1",
      "group_wait": "0s",
      "group_by": [
        "alertname"
      ],
      "routes": [
        {
          "receiver": "email_recv",
          "group_wait": "0s",
          "group_by": [
            "alertname"
          ],
          "matchers": [
            "alertname=\"EmailAlert\""
          ]
        },
        {
          "receiver": "slack_recv1",
          "group_wait": "0s",
          "group_by": [
            "alertname"
          ],
          "matchers": [
            "alertname=\"SlackAlert1\""
          ]
        },
        {
          "receiver": "slack_recv2",
          "group_wait": "0s",
          "group_by": [
            "alertname"
          ],
          "matchers": [
            "alertname=\"SlackAlert2\""
          ]
        },
        {
          "receiver": "pagerduty_recv",
          "group_wait": "0s",
          "group_by": [
            "alertname"
          ],
          "matchers": [
            "alertname=\"PagerdutyAlert\""
          ]
        },
        {
          "receiver": "dingding_recv",
          "group_wait": "0s",
          "group_by": [
            "alertname"
          ],
          "matchers": [
            "alertname=\"DingDingAlert\""
          ]
        },
        {
          "receiver": "teams_recv",
          "group_wait": "0s",
          "group_by": [
            "alertname"
          ],
          "matchers": [
            "alertname=\"TeamsAlert\""
          ]
        },
        {
          "receiver": "webhook_recv",
          "group_wait": "0s",
          "group_by": [
            "alertname"
          ],
          "matchers": [
            "alertname=\"WebhookAlert\""
          ]
        },
        {
          "receiver": "telegram_recv",
          "group_wait": "0s",
          "group_by": [
            "alertname"
          ],
          "matchers": [
            "alertname=\"TelegramAlert\""
          ]
        }
      ]
    },
    "templates": null,
    "receivers": [
      {
        "name": "email_recv",
        "grafana_managed_receiver_configs": [
          {
            "id": 0,
            "uid": "",
            "name": "email_test",
            "type": "email",
            "isDefault": false,
            "sendReminder": false,
            "disableResolveMessage": false,
            "frequency": "",
            "created": "0001-01-01T00:00:00Z",
            "updated": "0001-01-01T00:00:00Z",
            "settings": {
              "addresses": "test@email.com",
              "singleEmail": true
            },
            "secureFields": {}
          }
        ]
      },
      {
        "name": "dingding_recv",
        "grafana_managed_receiver_configs": [
          {
            "id": 0,
            "uid": "",
            "name": "dingding_test",
            "type": "dingding",
            "isDefault": false,
            "sendReminder": false,
            "disableResolveMessage": false,
            "frequency": "",
            "created": "0001-01-01T00:00:00Z",
            "updated": "0001-01-01T00:00:00Z",
            "settings": {
              "url": "http://CHANNEL_ADDR/dingding_recv/dingding_test"
            },
            "secureFields": {}
          }
        ]
      },
      {
        "name": "teams_recv",
        "grafana_managed_receiver_configs": [
          {
            "id": 0,
            "uid": "",
            "name": "teams_test",
            "type": "teams",
            "isDefault": false,
            "sendReminder": false,
            "disableResolveMessage": false,
            "frequency": "",
            "created": "0001-01-01T00:00:00Z",
            "updated": "0001-01-01T00:00:00Z",
            "settings": {
              "url": "http://CHANNEL_ADDR/teams_recv/teams_test"
            },
            "secureFields": {}
          }
        ]
      },
      {
        "name": "webhook_recv",
        "grafana_managed_receiver_configs": [
          {
            "id": 0,
            "uid": "",
            "name": "webhook_test",
            "type": "webhook",
            "isDefault": false,
            "sendReminder": false,
            "disableResolveMessage": false,
            "frequency": "",
            "created": "0001-01-01T00:00:00Z",
            "updated": "0001-01-01T00:00:00Z",
            "settings": {
              "url": "http://CHANNEL_ADDR/webhook_recv/webhook_test",
              "username": "my_username",
              "httpMethod": "POST",
              "maxAlerts": "5"
            },
            "secureFields": {
              "password": true
            }
          }
        ]
      },
      {
        "name": "telegram_recv",
        "grafana_managed_receiver_configs": [
          {
            "id": 0,
            "uid": "",
            "name": "telegram_test",
            "type": "telegram",
            "isDefault": false,
            "sendReminder": false,
            "disableResolveMessage": false,
            "frequency": "",
            "created": "0001-01-01T00:00:00Z",
            "updated": "0001-01-01T00:00:00Z",
            "settings": {
              "chatid": "telegram_chat_id"
            },
            "secureFields": {
              "bottoken": true
            }
          }
        ]
      },
      {
        "name": "slack_recv1",
        "grafana_managed_receiver_configs": [
          {
            "id": 0,
            "uid": "",
            "name": "slack_test_without_token",
            "type": "slack",
            "isDefault": false,
            "sendReminder": false,
            "disableResolveMessage": false,
            "frequency": "",
            "created": "0001-01-01T00:00:00Z",
            "updated": "0001-01-01T00:00:00Z",
            "settings": {
              "fallback": "Integration Test {{ template \"slack.default.title\" . }}",
              "icon_emoji": "🚀",
              "icon_url": "https://awesomeemoji.com/rocket",
              "mentionChannel": "here",
              "mentionGroups": "group1, group2",
              "mentionUsers": "user1, user2",
              "recipient": "#test-channel",
              "text": "Integration Test {{ template \"slack.default.text\" . }}",
              "title": "Integration Test {{ template \"slack.default.title\" . }}",
              "username": "Integration Test"
            },
            "secureFields": {
              "url": true
            }
          }
        ]
      },
      {
        "name": "slack_recv2",
        "grafana_managed_receiver_configs": [
          {
            "id": 0,
            "uid": "",
            "name": "slack_test_with_token",
            "type": "slack",
            "isDefault": false,
            "sendReminder": false,
            "disableResolveMessage": false,
            "frequency": "",
            "created": "0001-01-01T00:00:00Z",
            "updated": "0001-01-01T00:00:00Z",
            "settings": {
              "mentionUsers": "user1, user2",
              "recipient": "#test-channel",
              "username": "Integration Test"
            },
            "secureFields": {
              "token": true
            }
          }
        ]
      },
      {
        "name": "pagerduty_recv",
        "grafana_managed_receiver_configs": [
          {
            "id": 0,
            "uid": "",
            "name": "pagerduty_test",
            "type": "pagerduty",
            "isDefault": false,
            "sendReminder": false,
            "disableResolveMessage": false,
            "frequency": "",
            "created": "0001-01-01T00:00:00Z",
            "updated": "0001-01-01T00:00:00Z",
            "settings": {
              "class": "testclass",
              "component": "Integration Test",
              "group": "testgroup",
              "severity": "warning",
              "summary": "Integration Test {{ template \"pagerduty.default.description\" . }}"
            },
            "secureFields": {
              "integrationKey": true
            }
          }
        ]
      }
    ]
  }
}
`

// expNotifications is all the expected notifications.
// The key for the map is taken from the URL. The last 2 components of URL
// split with "/" forms the key for that route.
var expNotifications = map[string][]string{
	"slack_recv1/slack_test_without_token": {
		`{
		  "channel": "#test-channel",
		  "username": "Integration Test",
		  "icon_emoji": "🚀",
		  "icon_url": "https://awesomeemoji.com/rocket",
		  "attachments": [
			{
			  "title": "Integration Test [FIRING:1]  (SlackAlert1)",
			  "title_link": "TODO: rule URL",
			  "text": "Integration Test ",
			  "fallback": "Integration Test [FIRING:1]  (SlackAlert1)",
			  "footer": "Grafana v",
			  "footer_icon": "https://grafana.com/assets/img/fav32.png",
			  "color": "#D63232"
			}
		  ],
		  "blocks": [
			{
			  "text": {
				"text": "<!here|here> <!subteam^group1><!subteam^group2> <@user1><@user2>",
				"type": "mrkdwn"
			  },
			  "type": "section"
			}
		  ]
		}`,
	},
	"slack_recvX/slack_testX": {
		`{
		  "channel": "#test-channel",
		  "username": "Integration Test",
		  "attachments": [
			{
			  "title": "[FIRING:1]  (SlackAlert2)",
			  "title_link": "TODO: rule URL",
			  "text": "",
			  "fallback": "[FIRING:1]  (SlackAlert2)",
			  "footer": "Grafana v",
			  "footer_icon": "https://grafana.com/assets/img/fav32.png",
			  "color": "#D63232"
			}
		  ],
		  "blocks": [
			{
			  "text": {
				"text": "<@user1><@user2>",
				"type": "mrkdwn"
			  },
			  "type": "section"
			}
		  ]
		}`,
	},
	"pagerduty_recvX/pagerduty_testX": {
		`{
		  "routing_key": "pagerduty_recv/pagerduty_test",
		  "dedup_key": "718643b9694d44f7f2b21458afd1b079cb403cf264e51894ff3c9745238bcced",
		  "description": "[firing:1]  (PagerdutyAlert)",
		  "event_action": "trigger",
		  "payload": {
			"summary": "Integration Test [FIRING:1]  (PagerdutyAlert)",
			"source": "ganesh",
			"severity": "warning",
			"class": "testclass",
			"component": "Integration Test",
			"group": "testgroup",
			"custom_details": {
			  "firing": "Labels:\n - alertname = PagerdutyAlert\nAnnotations:\nSource: \n",
			  "num_firing": "1",
			  "num_resolved": "0",
			  "resolved": ""
			}
		  },
		  "client": "Grafana",
		  "client_url": "http://localhost:3000/",
		  "links": [
			{
			  "href": "http://localhost:3000/",
			  "text": "External URL"
			}
		  ]
		}`,
	},
	"dingding_recv/dingding_test": {
		`{
		 "link": {
			"messageUrl": "dingtalk://dingtalkclient/page/link?pc_slide=false&url=http%3A%2F%2Flocalhost%3A3000%2F",
			"text": "\n**Firing**\nLabels:\n - alertname = DingDingAlert\nAnnotations:\nSource: \n\n\n\n\n",
			"title": "[firing:1]  (DingDingAlert)"
		 },
		 "msgtype": "link"
		}`,
	},
	"teams_recv/teams_test": {
		`{
		  "@context": "http://schema.org/extensions",
		  "@type": "MessageCard",
		  "potentialAction": [
			{
			  "@context": "http://schema.org",
			  "@type": "OpenUri",
			  "name": "View Rule",
			  "targets": [
				{
				  "os": "default",
				  "uri": ""
				}
			  ]
			}
		  ],
		  "sections": [
			{
			  "text": "\n**Firing**\nLabels:\n - alertname = TeamsAlert\nAnnotations:\nSource: \n\n\n\n\n",
			  "title": "Details"
			}
		  ],
		  "summary": "[firing:1]  (TeamsAlert)",
		  "themeColor": "#D63232",
		  "title": "[firing:1]  (TeamsAlert)"
		}`,
	},
	"webhook_recv/webhook_test": {
		`{
		  "receiver": "webhook_recv",
		  "status": "firing",
		  "alerts": [
			{
			  "status": "firing",
			  "labels": {
				"alertname": "WebhookAlert"
			  },
			  "annotations": {},
			  "startsAt": "%s",
			  "endsAt": "0001-01-01T00:00:00Z",
			  "generatorURL": "",
			  "fingerprint": "a0e2a99fc56fce63"
			}
		  ],
		  "groupLabels": {},
		  "commonLabels": {
			"alertname": "WebhookAlert"
		  },
		  "commonAnnotations": {},
		  "externalURL": "http://localhost:3000/",
		  "version": "1",
		  "groupKey": "{}/{alertname=\"WebhookAlert\"}:{}",
		  "truncatedAlerts": 0,
		  "title": "[FIRING:1]  (WebhookAlert)",
		  "state": "alerting",
		  "message": "\n**Firing**\nLabels:\n - alertname = WebhookAlert\nAnnotations:\nSource: \n\n\n\n\n"
		}`,
	},
}

func getAlertmanagerConfig(channelAddr string) string {
	return strings.ReplaceAll(alertmanagerConfig, "CHANNEL_ADDR", channelAddr)
}

func getExpAlertmanagerConfigFromAPI(channelAddr string) string {
	return strings.ReplaceAll(expAlertmanagerConfigFromAPI, "CHANNEL_ADDR", channelAddr)
}

// alertNames are name of alerts to be sent. This should be in sync with
// the routes that we define in Alertmanager config.
// EmailAlert and TelegramAlert are missing because they don't
// send a JSON. Email and POST body are yet to be supported in the tests.
var alertNames = []string{"DingDingAlert", "SlackAlert1", "SlackAlert2", "PagerdutyAlert", "TeamsAlert", "WebhookAlert"}

func getRulesConfig(t *testing.T) string {
	interval, err := model.ParseDuration("10s")
	require.NoError(t, err)
	rules := apimodels.PostableRuleGroupConfig{
		Name:     "arulegroup",
		Interval: interval,
	}

	// Create rules that will fire as quickly as possible for all the routes.
	for _, alertName := range alertNames {
		rules.Rules = append(rules.Rules, apimodels.PostableExtendedRuleNode{
			GrafanaManagedAlert: &apimodels.PostableGrafanaRule{
				Title:     alertName,
				Condition: "A",
				Data: []ngmodels.AlertQuery{
					{
						RefID: "A",
						RelativeTimeRange: ngmodels.RelativeTimeRange{
							From: ngmodels.Duration(time.Duration(5) * time.Hour),
							To:   ngmodels.Duration(time.Duration(3) * time.Hour),
						},
						DatasourceUID: "-100",
						Model: json.RawMessage(`{
							"type": "math",
							"expression": "2 + 3 > 1"
						}`),
					},
				},
			},
		})
	}

	b, err := json.Marshal(rules)
	require.NoError(t, err)

	return string(b)
}

func TestNotificationChannels(t *testing.T) {
	dir, path := testinfra.CreateGrafDir(t, testinfra.GrafanaOpts{
		EnableFeatureToggles: []string{"ngalert"},
		DisableAnonymous:     true,
	})

	store := testinfra.SetUpDatabase(t, dir)
	store.Bus = bus.GetBus()
	grafanaListedAddr := testinfra.StartGrafana(t, dir, path, store)

	mockChannel := newMockNotificationChannel(t, grafanaListedAddr)
	amConfig := getAlertmanagerConfig(mockChannel.server.Addr)
	rulesConfig := getRulesConfig(t)

	// Overriding some URLs to send to the mock channel.
	channels.SlackAPIEndpoint = fmt.Sprintf("http://%s/slack_recvX/slack_testX", mockChannel.server.Addr)
	channels.PagerdutyEventAPIURL = fmt.Sprintf("http://%s/pagerduty_recvX/pagerduty_testX", mockChannel.server.Addr)
	channels.TelegramAPIURL = fmt.Sprintf("http://%s/telegram_recv/bot%%s", mockChannel.server.Addr)

	// Create a user to make authenticated requests
	require.NoError(t, createUser(t, store, models.ROLE_EDITOR, "grafana", "password"))

	{
		// There are no notification channel config initially.
		alertsURL := fmt.Sprintf("http://grafana:password@%s/api/alertmanager/grafana/config/api/v1/alerts", grafanaListedAddr)
		_ = getRequest(t, alertsURL, http.StatusNotFound) // nolint
	}

	{
		// Create the namespace we'll save our alerts to.
		require.NoError(t, createFolder(t, store, 0, "default"))

		// Post the alertmanager config.
		u := fmt.Sprintf("http://grafana:password@%s/api/alertmanager/grafana/config/api/v1/alerts", grafanaListedAddr)
		postRequest(t, u, amConfig, http.StatusAccepted)

		// Verifying that all the receivers and routes have been registered.
		alertsURL := fmt.Sprintf("http://grafana:password@%s/api/alertmanager/grafana/config/api/v1/alerts", grafanaListedAddr)
		resp := getRequest(t, alertsURL, http.StatusOK) // nolint
		b := getBody(t, resp.Body)
		require.JSONEq(t, getExpAlertmanagerConfigFromAPI(mockChannel.server.Addr), b)
	}

	{
		// Create rules that will fire as quickly as possible
		u := fmt.Sprintf("http://grafana:password@%s/api/ruler/grafana/api/v1/rules/default", grafanaListedAddr)
		postRequest(t, u, rulesConfig, http.StatusAccepted)
	}

	// Eventually, we'll get all the desired alerts.
	// nolint:gosec
	require.Eventually(t, func() bool {
		return mockChannel.totalNotifications() == len(alertNames) &&
			mockChannel.matchesExpNotifications(expNotifications)
	}, 30*time.Second, 1*time.Second)

	require.NoError(t, mockChannel.Close())
}

func getRequest(t *testing.T, url string, expStatusCode int) *http.Response {
	// nolint:gosec
	resp, err := http.Get(url)
	t.Cleanup(func() {
		require.NoError(t, resp.Body.Close())
	})
	require.NoError(t, err)
	require.Equal(t, expStatusCode, resp.StatusCode)
	return resp
}

func postRequest(t *testing.T, url string, body string, expStatusCode int) {
	buf := bytes.NewReader([]byte(body))
	// nolint:gosec
	resp, err := http.Post(url, "application/json", buf)
	t.Cleanup(func() {
		require.NoError(t, resp.Body.Close())
	})
	require.NoError(t, err)
	require.Equal(t, expStatusCode, resp.StatusCode)
}

type mockNotificationChannel struct {
	t      *testing.T
	server *http.Server

	receivedNotifications    map[string][]string
	receivedNotificationsMtx sync.Mutex
}

func newMockNotificationChannel(t *testing.T, grafanaListedAddr string) *mockNotificationChannel {
	lastDigit := grafanaListedAddr[len(grafanaListedAddr)-1] - 48
	lastDigit = (lastDigit + 1) % 10
	newAddr := fmt.Sprintf("%s%01d", grafanaListedAddr[:len(grafanaListedAddr)-1], lastDigit)

	nc := &mockNotificationChannel{
		server: &http.Server{
			Addr: newAddr,
		},
		receivedNotifications: make(map[string][]string),
		t:                     t,
	}

	nc.server.Handler = nc
	go func() {
		require.Equal(t, http.ErrServerClosed, nc.server.ListenAndServe())
	}()

	return nc
}

func (nc *mockNotificationChannel) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	nc.receivedNotificationsMtx.Lock()
	defer nc.receivedNotificationsMtx.Unlock()

	urlParts := strings.Split(req.URL.String(), "/")
	key := fmt.Sprintf("%s/%s", urlParts[len(urlParts)-2], urlParts[len(urlParts)-1])
	body := getBody(nc.t, req.Body)

	// Special handling for non-static data, like timestamp.
	if strings.Contains(key, "slack") {
		// Remove the timestamp from slack.
		j, err := simplejson.NewJson([]byte(body))
		require.NoError(nc.t, err)

		attachments, err := j.Get("attachments").Array()
		require.NoError(nc.t, err)

		aj := simplejson.NewFromAny(attachments[0])
		aj.Del("ts")
		j.Set("attachments", []interface{}{aj.Interface()})

		b, err := j.Encode()
		require.NoError(nc.t, err)
		body = string(b)
	}

	nc.receivedNotifications[key] = append(nc.receivedNotifications[key], body)
	res.WriteHeader(http.StatusOK)
}

func getBody(t *testing.T, body io.ReadCloser) string {
	b, err := ioutil.ReadAll(body)
	require.NoError(t, err)
	return string(b)
}

func (nc *mockNotificationChannel) totalNotifications() int {
	total := 0
	nc.receivedNotificationsMtx.Lock()
	defer nc.receivedNotificationsMtx.Unlock()
	for _, v := range nc.receivedNotifications {
		total += len(v)
	}
	return total
}

func (nc *mockNotificationChannel) matchesExpNotifications(exp map[string][]string) bool {
	nc.receivedNotificationsMtx.Lock()
	defer nc.receivedNotificationsMtx.Unlock()

	if len(nc.receivedNotifications) != len(exp) {
		return false
	}

	for expKey, expVals := range exp {
		actVals, ok := nc.receivedNotifications[expKey]
		if !ok || len(actVals) != len(expVals) {
			return false
		}
		for i := range expVals {
			expVal := expVals[i]
			if expKey == "webhook_recv/webhook_test" {
				// Special case since it has a time component "startsAt".
				r := regexp.MustCompile(`.*"startsAt"\s*:\s*"([^"]+)"`)
				parts := r.FindStringSubmatch(actVals[i])
				require.Equal(nc.t, 2, len(parts))
				expVal = fmt.Sprintf(expVal, parts[1])
			}

			var expJson, actJson interface{}
			require.NoError(nc.t, json.Unmarshal([]byte(expVal), &expJson))
			require.NoError(nc.t, json.Unmarshal([]byte(actVals[i]), &actJson))
			if !assert.ObjectsAreEqual(expJson, actJson) {
				return false
			}
		}
	}

	return true
}

func (nc *mockNotificationChannel) Close() error {
	return nc.server.Close()
}
