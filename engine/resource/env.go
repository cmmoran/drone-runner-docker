package resource

type Variable struct {
	Value      string `json:"value,omitempty"`
	Secret     string `json:"from_secret,omitempty" yaml:"from_secret"`
	FromOutput string `json:"from_output,omitempty" yaml:"from_output"`
}

type variable struct {
	Value      string
	Secret     string `yaml:"from_secret"`
	FromOutput string `yaml:"from_output"`
}

func (v *Variable) UnmarshalYAML(unmarshal func(interface{}) error) error {
	d := new(variable)
	err := unmarshal(&d.Value)
	if err != nil {
		err = unmarshal(d)
	}
	v.Value = d.Value
	v.Secret = d.Secret
	v.FromOutput = d.FromOutput
	return err
}

func (v *Variable) MarshalYAML() (interface{}, error) {
	if v.FromOutput != "" {
		return map[string]interface{}{"from_output": v.FromOutput}, nil
	}
	if v.Secret != "" {
		return map[string]interface{}{"from_secret": v.Secret}, nil
	}
	if v.Value != "" {
		return v.Value, nil
	}
	return nil, nil
}
