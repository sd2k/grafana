// Libraries
import { toString, toNumber as _toNumber, isEmpty, isBoolean } from 'lodash';

// Types
import { Field, FieldType } from '../types/dataFrame';
import { DisplayProcessor, DisplayValue } from '../types/displayValue';
import { getValueFormat } from '../valueFormats/valueFormats';
import { getValueMappingResult } from '../utils/valueMappings';
import { dateTime } from '../datetime';
import { KeyValue, TimeZone } from '../types';
import { getScaleCalculator, ScaleCalculator } from './scale';
import { GrafanaTheme2 } from '../themes/types';
import { anyToNumber } from '../utils/anyToNumber';
import { classicColors, getColorForTheme } from '../utils/namedColorsPalette';

interface DisplayProcessorOptions {
  field: Partial<Field>;
  /**
   * Will pick browser timezone if not defined
   */
  timeZone?: TimeZone;
  /**
   * Will pick 'dark' if not defined
   */
  theme: GrafanaTheme2;
}

// Reasonable units for time
const timeFormats: KeyValue<boolean> = {
  dateTimeAsIso: true,
  dateTimeAsIsoNoDateIfToday: true,
  dateTimeAsUS: true,
  dateTimeAsUSNoDateIfToday: true,
  dateTimeAsLocal: true,
  dateTimeAsLocalNoDateIfToday: true,
  dateTimeFromNow: true,
};

export function getDisplayProcessor(options?: DisplayProcessorOptions): DisplayProcessor {
  if (!options || isEmpty(options) || !options.field) {
    return toStringProcessor;
  }

  const field = options.field as Field;
  const config = field.config ?? {};

  let unit = config.unit;
  let hasDateUnit = unit && (timeFormats[unit] || unit.startsWith('time:'));

  if (field.type === FieldType.time && !hasDateUnit) {
    unit = `dateTimeAsSystem`;
    hasDateUnit = true;
  }

  const formatFunc = getValueFormat(unit || 'none');
  const scaleFunc = getScaleCalculator(field, options.theme);
  const defaultColor = getDefaultColorFunc(field, scaleFunc, options.theme);

  return (value: any) => {
    const { mappings } = config;
    const isStringUnit = unit === 'string';

    if (hasDateUnit && typeof value === 'string') {
      value = dateTime(value).valueOf();
    }

    let text = toString(value);
    let numeric = isStringUnit ? NaN : anyToNumber(value);
    let prefix: string | undefined = undefined;
    let suffix: string | undefined = undefined;
    let color: string | undefined = undefined;
    let percent: number | undefined = undefined;

    let shouldFormat = true;

    if (mappings && mappings.length > 0) {
      const mappingResult = getValueMappingResult(mappings, value);

      if (mappingResult) {
        if (mappingResult.text != null) {
          text = mappingResult.text;
        }

        if (mappingResult.color != null) {
          color = getColorForTheme(mappingResult.color, options.theme.v1);
        }

        shouldFormat = false;
      }
    }

    if (!isNaN(numeric)) {
      if (shouldFormat && !isBoolean(value)) {
        const v = formatFunc(numeric, config.decimals, null, options.timeZone);
        text = v.text;
        suffix = v.suffix;
        prefix = v.prefix;
      }

      // Return the value along with scale info
      if (color === undefined) {
        const scaleResult = scaleFunc(numeric);
        color = scaleResult.color;
        percent = scaleResult.percent;
      }
    }

    if (!text) {
      if (config.noValue) {
        text = config.noValue;
      } else {
        text = ''; // No data?
      }
    }

    if (!color) {
      const scaleResult = defaultColor(value);
      color = scaleResult.color;
      percent = scaleResult.percent;
    }

    return { text, numeric, prefix, suffix, color, percent };
  };
}

function toStringProcessor(value: any): DisplayValue {
  return { text: toString(value), numeric: anyToNumber(value) };
}

export function getRawDisplayProcessor(): DisplayProcessor {
  return (value: any) => ({
    text: `${value}`,
    numeric: (null as unknown) as number,
  });
}

function getDefaultColorFunc(field: Field, scaleFunc: ScaleCalculator, theme: GrafanaTheme2) {
  if (field.type === FieldType.string) {
    return (value: any) => {
      const hc = strHashCode(value as string);
      return {
        color: classicColors[Math.floor(hc % classicColors.length)],
        percent: 0,
      };
    };
  }
  return (value: any) => scaleFunc(-Infinity);
}

/**
 * @internal
 */
function strHashCode(str: string) {
  return str.split('').reduce((prevHash, currVal) => ((prevHash << 5) - prevHash + currVal.charCodeAt(0)) | 0, 0);
}
