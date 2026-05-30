#!/usr/bin/env python3
import os
import secrets
import string
from pathlib import Path

# Configuration
CONFIG_DIR = Path("config")
SECRETS_FILE = Path(".secrets.env")

TEMPLATES = [
    CONFIG_DIR / "seaweedfs/s3.json.template",
    CONFIG_DIR / "loki/config.yaml.template",
    CONFIG_DIR / "tempo/config.yaml.template",
    CONFIG_DIR / "thanos/bucket.yaml.template",
]

def generate_secret(length=32):
    """Generate a random alphanumeric string."""
    alphabet = string.ascii_letters + string.digits
    return ''.join(secrets.choice(alphabet) for _ in range(length))

def load_or_generate_secrets():
    """Load secrets from file or generate new ones."""
    secrets_dict = {}
    
    if SECRETS_FILE.exists():
        print(f"Loading existing secrets from {SECRETS_FILE}")
        with open(SECRETS_FILE, "r") as f:
            for line in f:
                line = line.strip()
                if line and not line.startswith("#") and "=" in line:
                    key, value = line.split("=", 1)
                    secrets_dict[key] = value
    else:
        print("Generating new secrets...")
        keys = [
            "LOKI_ACCESS_KEY", "LOKI_SECRET_KEY",
            "TEMPO_ACCESS_KEY", "TEMPO_SECRET_KEY",
            "THANOS_ACCESS_KEY", "THANOS_SECRET_KEY",
            "JWT_SECRET",
        ]
        
        with open(SECRETS_FILE, "w") as f:
            for key in keys:
                value = generate_secret()
                secrets_dict[key] = value
                f.write(f"{key}={value}\n")
        print(f"Secrets saved to {SECRETS_FILE}")
            
    return secrets_dict

def process_templates(secrets_dict):
    """Replace placeholders in templates with secret values."""
    for template_path in TEMPLATES:
        if not template_path.exists():
            print(f"Warning: Template {template_path} not found.")
            continue
            
        output_path = template_path.with_suffix("") # Remove .template extension
        print(f"Generating {output_path} from {template_path}...")
        
        with open(template_path, "r") as f:
            content = f.read()
            
        # Perform substitution
        # We use strict substitution where every placeholder must exist in secrets
        # Using string.Template would act similarly but we can just use replace for simplicity 
        # as we have specific placeholders like ${KEY}
        
        for key, value in secrets_dict.items():
            placeholder = f"${{{key}}}"
            content = content.replace(placeholder, value)
            
        with open(output_path, "w") as f:
            f.write(content)

def main():
    secrets_dict = load_or_generate_secrets()
    process_templates(secrets_dict)
    print("Configuration generation complete.")

if __name__ == "__main__":
    main()
