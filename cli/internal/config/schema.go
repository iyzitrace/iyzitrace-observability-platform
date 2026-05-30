// Package config models iyzitrace.yaml — a slim knobs file. The shape on
// disk is the source of truth; we parse it into the typed Config below for
// `apply` to materialize `.env`, `.secrets.env`, and to template files in
// config/. Per-service tuning (retention, scrape interval, etc.) lives in
// the rendered config/ files themselves, not in iyzitrace.yaml.
package config

type Config struct {
	Deployment Deployment         `yaml:"deployment"`
	Secrets    Secrets            `yaml:"secrets"`
	Services   map[string]Service `yaml:"services"`
}

type Deployment struct {
	Network   string `yaml:"network"`
	DataDir   string `yaml:"data_dir"`
	HTTPPort  int    `yaml:"http_port"`
	HTTPSPort int    `yaml:"https_port"`
	Domain    string `yaml:"domain"`
}

type Secrets struct {
	Backend         string `yaml:"backend"` // file | vault (v2)
	RotateOnUpgrade bool   `yaml:"rotate_on_upgrade"`
}

type Service struct {
	Image string `yaml:"image,omitempty"` // takes precedence if set
	Build string `yaml:"build,omitempty"` // host path used as `build:` context when Image is empty
}
