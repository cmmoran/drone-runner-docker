package resource

type Parameter struct {
	Value      interface{} `json:"value,omitempty"`
	Secret     string      `json:"from_secret,omitempty" yaml:"from_secret"`
	FromOutput string      `json:"from_output,omitempty" yaml:"from_output"`
}

type parameter struct {
	Value      interface{}
	Secret     string `yaml:"from_secret"`
	FromOutput string `yaml:"from_output"`
}

func (p *Parameter) UnmarshalYAML(unmarshal func(interface{}) error) error {
	d := new(parameter)
	if err := unmarshal(d); err == nil && (d.Secret != "" || d.FromOutput != "") {
		p.Value = d.Value
		p.Secret = d.Secret
		p.FromOutput = d.FromOutput
		return nil
	}
	var raw interface{}
	if err := unmarshal(&raw); err != nil {
		return err
	}
	p.Value = raw
	p.Secret = ""
	p.FromOutput = ""
	return nil
}

func (p *Parameter) MarshalYAML() (interface{}, error) {
	if p.FromOutput != "" {
		return map[string]interface{}{"from_output": p.FromOutput}, nil
	}
	if p.Secret != "" {
		return map[string]interface{}{"from_secret": p.Secret}, nil
	}
	if p.Value != nil {
		return p.Value, nil
	}
	return nil, nil
}
