package config

import (
	"errors"
	"fmt"
	"strings"
)

func (c *Config) Validate() error {
	var errs []string

	if c.Deployment.HTTPPort <= 0 || c.Deployment.HTTPPort > 65535 {
		errs = append(errs, fmt.Sprintf("deployment.http_port %d out of range", c.Deployment.HTTPPort))
	}
	if c.Deployment.HTTPSPort <= 0 || c.Deployment.HTTPSPort > 65535 {
		errs = append(errs, fmt.Sprintf("deployment.https_port %d out of range", c.Deployment.HTTPSPort))
	}
	if c.Deployment.HTTPPort == c.Deployment.HTTPSPort {
		errs = append(errs, "deployment.http_port and deployment.https_port must differ")
	}
	if c.Deployment.DataDir == "" {
		errs = append(errs, "deployment.data_dir must be set")
	}
	if c.Deployment.Network == "" {
		errs = append(errs, "deployment.network must be set")
	}
	if c.Secrets.Backend != "file" {
		errs = append(errs, fmt.Sprintf("secrets.backend %q: only \"file\" is supported in v1", c.Secrets.Backend))
	}
	for name, svc := range c.Services {
		if svc.Image == "" && svc.Build == "" {
			errs = append(errs, fmt.Sprintf("services.%s: one of image or build is required", name))
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errors.New("invalid config:\n  - " + strings.Join(errs, "\n  - "))
}
