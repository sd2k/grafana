import React, { useMemo } from 'react';
import { FieldOverrideEditorProps, FieldType, getFieldDisplayName, SelectableValue } from '@grafana/data';
import { Input, Select } from '@grafana/ui';

export const FillBellowToEditor: React.FC<FieldOverrideEditorProps<string, any>> = ({ value, context, onChange }) => {
  const names = useMemo(() => {
    const names: Array<SelectableValue<string>> = [];
    if (context.data.length) {
      for (const frame of context.data) {
        for (const field of frame.fields) {
          if (field.type === FieldType.number) {
            const label = getFieldDisplayName(field, frame, context.data);
            names.push({
              label,
              value: label,
            });
          }
        }
      }
    }
    return names;
  }, [context]);

  const current = useMemo(() => {
    const found = names.find((v) => v.value === value);
    if (found) {
      return found;
    }
    if (value) {
      return {
        label: value,
        value,
      };
    }
    return undefined;
  }, [names, value]);

  return (
    <Select
      options={names}
      value={current}
      onChange={(v) => {
        onChange(v.value);
      }}
    />
  );
};

interface FillBelowToRegexpValue {
  from?: string;
  to?: string;
}

export const FillBelowToRegexpEditor: React.FC<FieldOverrideEditorProps<FillBelowToRegexpValue, any>> = ({
  value,
  context,
  onChange,
}) => (
  <>
    <Input
      placeholder="From"
      value={value?.from}
      onChange={(v) => onChange({ ...value, from: v.currentTarget.value })}
    />
    <Input placeholder="To" value={value?.to} onChange={(v) => onChange({ ...value, to: v.currentTarget.value })} />
  </>
);
