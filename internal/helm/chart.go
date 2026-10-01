package helm

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ChartMeta is the part of a Chart.yaml that this package needs. It is
// deliberately minimal and local: package normalize owns full Chart.yaml
// handling.
type ChartMeta struct {
	APIVersion  string `yaml:"apiVersion"`
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	AppVersion  string `yaml:"appVersion"`
	KubeVersion string `yaml:"kubeVersion"`
	Type        string `yaml:"type"`
	Deprecated  bool   `yaml:"deprecated"`
}

// ParseChartYAML decodes a Chart.yaml document. Scalars such as
// "appVersion: 1.10" are kept verbatim as strings.
func ParseChartYAML(b []byte) (*ChartMeta, error) {
	var m ChartMeta
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("decode Chart.yaml: %w", err)
	}
	if m.Name == "" && m.Version == "" {
		return nil, fmt.Errorf("decode Chart.yaml: neither name nor version present")
	}
	return &m, nil
}
