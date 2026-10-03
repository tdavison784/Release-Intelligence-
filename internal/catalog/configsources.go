package catalog

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// ConfigChannel names how a product reads (part of) its configuration. The
// list is generic: a product declares which channels it uses, as data, so the
// learning loop knows which declarative predicate can test "is setting X
// configured here" (LOOP-DIAGNOSIS-2 lever L1). No product-specific logic.
type ConfigChannel string

const (
	// ChannelConfigMapFile: configuration supplied in a ConfigMap: one config
	// file embedded as a data key (File + Format; tested by text-line), or,
	// without File, one setting per data key (ConfigMap required; tested by
	// field data.<key>).
	ChannelConfigMapFile ConfigChannel = "configmap-file"
	// ChannelConfigFile: a config file read from disk that is not
	// conventionally a ConfigMap (VMs, images).
	ChannelConfigFile ConfigChannel = "config-file"
	// ChannelHelmValues: the official chart's values (ValuesPath = prefix).
	ChannelHelmValues ConfigChannel = "helm-values"
	// ChannelCLIFlags: command-line flags of a binary (Component).
	ChannelCLIFlags ConfigChannel = "cli-flags"
	// ChannelEnvVars: environment variables the binary reads as configuration.
	ChannelEnvVars ConfigChannel = "env-vars"
	// ChannelFeatureGates: a feature-gate mechanism (Flag, ValuesPath).
	ChannelFeatureGates ConfigChannel = "feature-gates"
	// ChannelCustomResource: configuration read from the product's own custom
	// resource (Resource).
	ChannelCustomResource ConfigChannel = "custom-resource"
)

// ConfigChannels lists every channel.
var ConfigChannels = []ConfigChannel{
	ChannelConfigMapFile, ChannelConfigFile, ChannelHelmValues, ChannelCLIFlags,
	ChannelEnvVars, ChannelFeatureGates, ChannelCustomResource,
}

// ConfigFormats are the file formats a config file may declare.
var ConfigFormats = []string{"yaml", "json", "toml", "ini", "text", "csv", "hcl", "conf"}

// ConfigSource is one place a product reads configuration from, stated by
// upstream documentation (References are required: a config source is an
// upstream fact, never inferred from an environment or an evaluation case).
type ConfigSource struct {
	ID      string        `yaml:"id" json:"id"`
	Channel ConfigChannel `yaml:"channel" json:"channel"`
	// Component is the binary / container / workload that reads it.
	Component string `yaml:"component,omitempty" json:"component,omitempty"`
	// Summary says what is configured here, in one sentence.
	Summary string `yaml:"summary" json:"summary"`
	// File is the file name or ConfigMap data key (configmap-file, config-file).
	File string `yaml:"file,omitempty" json:"file,omitempty"`
	// Format of the file (configmap-file, config-file).
	Format string `yaml:"format,omitempty" json:"format,omitempty"`
	// ConfigMap is the conventional ConfigMap name (configmap-file).
	ConfigMap string `yaml:"configMap,omitempty" json:"configMap,omitempty"`
	// ValuesPath is the chart values key: the prefix of product settings
	// (helm-values; "." = chart root), or the key that sets this source
	// (feature-gates, configmap-file).
	ValuesPath string `yaml:"valuesPath,omitempty" json:"valuesPath,omitempty"`
	// Flag carries the gate list (feature-gates): a command-line flag
	// ("--feature-gates") or an environment variable ("STRIMZI_FEATURE_GATES").
	Flag string `yaml:"flag,omitempty" json:"flag,omitempty"`
	// Resource is the custom resource the product reads (custom-resource).
	Resource *ConfigResource `yaml:"resource,omitempty" json:"resource,omitempty"`
	// References are the upstream documentation the entry rests on.
	References []ConfigReference `yaml:"references" json:"references"`
}

// ConfigResource identifies a custom resource kind.
type ConfigResource struct {
	Group string `yaml:"group,omitempty" json:"group,omitempty"`
	Kind  string `yaml:"kind" json:"kind"`
}

// ConfigReference cites upstream documentation: a URL and a short verbatim
// quote from it.
type ConfigReference struct {
	URL   string `yaml:"url" json:"url"`
	Quote string `yaml:"quote" json:"quote"`
}

var envVarNameRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

var configSourceIDRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// maxConfigQuote bounds a reference quote (a citation, not a copy).
const maxConfigQuote = 400

func (v *validator) configSources(d *ProductDefinition) {
	seen := map[string]bool{}
	for i, s := range d.ConfigSources {
		p := fmt.Sprintf("configSources[%d]", i)
		switch {
		case !configSourceIDRe.MatchString(s.ID):
			v.errf(p+".id", "must be a lowercase DNS label, got %q", s.ID)
		case seen[s.ID]:
			v.errf(p+".id", "duplicate config source id %q", s.ID)
		}
		seen[s.ID] = true
		known := false
		for _, c := range ConfigChannels {
			known = known || c == s.Channel
		}
		if !known {
			v.errf(p+".channel", "unknown channel %q (one of %v)", s.Channel, ConfigChannels)
		}
		if strings.TrimSpace(s.Summary) == "" {
			v.errf(p+".summary", "required")
		}
		// channel-specific fields: required ones present, foreign ones absent
		need := map[ConfigChannel][]string{
			ChannelConfigFile: {"file", "format"},
			ChannelHelmValues:     {"valuesPath"},
			ChannelCustomResource: {"resource"},
		}[s.Channel]
		allowed := map[ConfigChannel][]string{
			ChannelConfigMapFile:  {"file", "format", "configMap", "valuesPath"},
			ChannelConfigFile:     {"file", "format"},
			ChannelHelmValues:     {"valuesPath"},
			ChannelCLIFlags:       nil,
			ChannelEnvVars:        nil,
			ChannelFeatureGates:   {"flag", "valuesPath"},
			ChannelCustomResource: {"resource"},
		}[s.Channel]
		set := map[string]bool{
			"file": s.File != "", "format": s.Format != "", "configMap": s.ConfigMap != "",
			"valuesPath": s.ValuesPath != "", "flag": s.Flag != "", "resource": s.Resource != nil,
		}
		for _, f := range need {
			if !set[f] {
				v.errf(p+"."+f, "required for channel %s", s.Channel)
			}
		}
		if known {
			for f, isSet := range set {
				if isSet && !containsStr(allowed, f) {
					v.errf(p+"."+f, "not a field of channel %s", s.Channel)
				}
			}
		}
		if s.Channel == ChannelConfigMapFile {
			switch {
			case s.File != "" && s.Format == "":
				v.errf(p+".format", "required with file (the embedded file's format)")
			case s.File == "" && s.ConfigMap == "":
				v.errf(p+".configMap", "required when settings are the ConfigMap's data keys (no file)")
			case s.File == "" && s.Format != "":
				v.errf(p+".format", "only with file")
			}
		}
		if s.Format != "" && !containsStr(ConfigFormats, s.Format) {
			v.errf(p+".format", "unknown format %q (one of %v)", s.Format, ConfigFormats)
		}
		if s.Flag != "" && !strings.HasPrefix(s.Flag, "-") && !envVarNameRe.MatchString(s.Flag) {
			v.errf(p+".flag", "must be a flag (leading -) or an environment variable name, got %q", s.Flag)
		}
		if s.Resource != nil && strings.TrimSpace(s.Resource.Kind) == "" {
			v.errf(p+".resource.kind", "required")
		}
		if len(s.References) == 0 {
			v.errf(p+".references", "at least one upstream documentation reference is required")
		}
		for j, r := range s.References {
			rp := fmt.Sprintf("%s.references[%d]", p, j)
			u, err := url.Parse(r.URL)
			if err != nil || u.Scheme != "https" || u.Host == "" {
				v.errf(rp+".url", "must be an https URL of upstream documentation, got %q", r.URL)
			}
			// knowledge is release-level and authored blind: a config source
			// never cites this repository's evaluation data
			if strings.Contains(r.URL, "eval/cases") || strings.Contains(r.URL, "eval/results") {
				v.errf(rp+".url", "must not cite evaluation data")
			}
			if q := strings.TrimSpace(r.Quote); q == "" || len(q) > maxConfigQuote {
				v.errf(rp+".quote", "a short verbatim quote (1-%d characters) is required", maxConfigQuote)
			}
		}
	}
}

func containsStr(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
