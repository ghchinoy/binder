## binder config unset

Remove a persistent configuration value

### Synopsis

Unset removes a key from ./.binder.yaml (default) or ~/.config/binder/config.yaml
(with --global), reverting the setting to its environment override or built-in default.

```
binder config unset <key> [flags]
```

### Options

```
  -g, --global   remove setting from global user config (~/.config/binder/config.yaml) instead of ./.binder.yaml
  -h, --help     help for unset
      --json     emit the result as JSON (schema binder.config/v1)
```

### SEE ALSO

* [binder config](binder_config.md)	 - Manage configuration (show, get, set, unset)

