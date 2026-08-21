## binder config set

Set a persistent configuration value in .binder.yaml or user config

### Synopsis

Set persists a configuration setting to ./.binder.yaml (default) or
~/.config/binder/config.yaml (with --global).

It performs isolated file mutation, modifying only the specified key
without altering other settings or dumping runtime defaults.

```
binder config set <key> <value> [flags]
```

### Options

```
  -g, --global   write setting to global user config (~/.config/binder/config.yaml) instead of ./.binder.yaml
  -h, --help     help for set
      --json     emit the result as JSON (schema binder.config/v1)
```

### SEE ALSO

* [binder config](binder_config.md)	 - Manage configuration (show, get, set, unset)

