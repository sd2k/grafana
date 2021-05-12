import { FieldColorModeId, FieldConfigProperty, PanelPlugin, ReducerID, standardEditorsRegistry } from '@grafana/data';
import { PieChartPanel } from './PieChartPanel';
import { PieChartOptions, PieChartType, PieChartLabels, PieChartLegendValues } from './types';
import { LegendDisplayMode, commonOptionsBuilder } from '@grafana/ui';
import { PieChartPanelChangedHandler } from './migrations';

export const plugin = new PanelPlugin<PieChartOptions>(PieChartPanel)
  .setPanelChangeHandler(PieChartPanelChangedHandler)
  .useFieldConfig({
    disableStandardOptions: [FieldConfigProperty.Thresholds],
    standardOptions: {
      [FieldConfigProperty.Color]: {
        settings: {
          byValueSupport: false,
          bySeriesSupport: true,
          preferThresholdsMode: false,
        },
        defaultValue: {
          mode: FieldColorModeId.PaletteClassic,
        },
      },
    },
    useCustomConfig: (builder) => {
      commonOptionsBuilder.addHideFrom(builder);
    },
  })
  .setPanelOptions((builder) => {
    builder
      .addCustomEditor({
        id: 'reduceOptions.calcs',
        path: 'reduceOptions.calcs',
        name: 'Calculation',
        description: 'Choose a reducer function / calculation',
        editor: standardEditorsRegistry.get('stats-picker').editor as any,
        defaultValue: [ReducerID.lastNotNull],
        // Hides it when all values mode is on
        showIf: (currentConfig) => currentConfig.reduceOptions.values === false,
      })
      .addRadio({
        name: 'Piechart type',
        description: 'How the piechart should be rendered',
        path: 'pieType',
        settings: {
          options: [
            { value: PieChartType.Pie, label: 'Pie' },
            { value: PieChartType.Donut, label: 'Donut' },
          ],
        },
        defaultValue: PieChartType.Pie,
      })
      .addMultiSelect({
        name: 'Labels',
        path: 'displayLabels',
        description: 'Select the labels to be displayed in the pie chart',
        settings: {
          options: [
            { value: PieChartLabels.Percent, label: 'Percent' },
            { value: PieChartLabels.Name, label: 'Name' },
            { value: PieChartLabels.Value, label: 'Value' },
          ],
        },
      });

    commonOptionsBuilder.addTooltipOptions(builder);
    commonOptionsBuilder.addLegendOptions(builder, false);

    builder.addMultiSelect({
      name: 'Legend values',
      path: 'legend.values',
      category: ['Legend'],
      settings: {
        options: [
          { value: PieChartLegendValues.Percent, label: 'Percent' },
          { value: PieChartLegendValues.Value, label: 'Value' },
        ],
      },
      showIf: (c) => c.legend.displayMode !== LegendDisplayMode.Hidden,
    });
  });
