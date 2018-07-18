put cni conf in `/etc/cni/net.d/`

run
```
CNI_PATH=${cni_plugin_bin}
CNI_PATH=$CNI_PATH ./priv-net-run.sh [command]
```
