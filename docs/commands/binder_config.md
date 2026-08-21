## binder config

Manage configuration (show, get, set, unset)

### Synopsis

Config manages persistent settings and prints the resolved effective
configuration: each key's value and its source (flag, env, file, or default),
plus the config file that was read (if any). Precedence is flag > env >
config file > built-in default. Ships --json (schema binder.config/v1).

```
binder config [flags]
```

### Options

```
  -h, --help   help for config
      --json   emit the resolved config as deterministic JSON (schema binder.config/v1)
```

### SEE ALSO

* [binder](binder.md)	 - Convert a plain-markdown corpus into a conformant OKF v0.2 bundle
* [binder config get](binder_config_get.md)	 - Get the resolved value of a configuration key
* [binder config list](binder_config_list.md)	 - List all resolved configuration values and their sources
* [binder config set](binder_config_set.md)	 - Set a persistent configuration value in .binder.yaml or user config
* [binder config unset](binder_config_unset.md)	 - Remove a persistent configuration value

