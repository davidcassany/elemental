package dynamicdata

import (
	"fmt"

	"go.yaml.in/yaml/v3"
)

type Data struct {
	Source string
	Values map[string]any
}

func Parse(raw []byte, source string) (*Data, error) {
	values := map[string]any{}
	if err := yaml.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("parsing Dynamic Node User Data: %w", err)
	}
	return &Data{Source: source, Values: values}, nil
}

func (d *Data) String(key string) string {
	if d == nil {
		return ""
	}
	value, _ := d.Values[key].(string)
	return value
}

func (d *Data) Map(key string) map[string]any {
	if d == nil {
		return nil
	}
	value, _ := d.Values[key].(map[string]any)
	return value
}
