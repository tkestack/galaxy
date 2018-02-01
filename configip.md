# 套件FloatingIP配置

## 1. 手写IP配置

### 套件配置apiswitch ip池

![image](doc/apiswitch-ip.png)

```
"ipam":{
	"floatingips": [{
		"routableSubnet": "10.56.19.192/26",
		"ips": ["10.56.19.238~10.56.19.247"],
		"subnet": "10.56.19.192/26",
		"gateway": "10.56.19.193"
	}, {
		"routableSubnet": "10.175.106.128/26",
		"ips": ["10.175.198.2~10.175.198.3", "10.175.198.8~10.175.198.11", "10.175.198.23~10.175.198.184"],
		"subnet": "10.175.198.0/24",
		"gateway": "10.175.198.1",
		"vlan": 3
	}]
}
```

### 套件配置kube-scheduler rackfilter配置项

![image](doc/scheduler-policy-file.png)

scheduler.own.args增加参数：

```
--policy-config-file=/etc/kubernetes/kube-scheduler-policy-config.json
```

![image](doc/scheduler-policy.png)

scheduler-policy-env-template配置框中填入以下样例

```
{
	"kind": "Policy",
	"apiVersion": "v1",
	"predicates": [
		{"name": "NoDiskConflict"}, 
		{"name": "NoVolumeZoneConflict"}, 
		{"name": "PodFitsHostPorts"}, 
		{"name": "PodFitsResources"}, 
		{"name": "PodFitsLocalDisk"}, 
		{"name": "HostName"}, 
		{"name": "MatchNodeSelector"}, 
		{"name": "PodToleratesNodeTaints"}, 
		{"name": "CheckNodeMemoryPressure"}, 
		{"name": "CheckNodeDiskPressure"}, 
		{"name": "MatchInterPodAffinity"}
	],
	"priorities": [
		{"name": "LeastRequestedPriority", "weight": 1}, 
		{"name": "BalancedResourceAllocation", "weight": 1}, 
		{"name": "ServiceSpreadingPriority", "weight": 1}, 
		{"name": "NodePreferAvoidPodsPriority", "weight": 10000}, 
		{"name": "NodeAffinityPriority", "weight": 1}, 
		{"name": "TaintTolerationPriority", "weight": 1}, 
		{"name": "InterPodAffinityPriority", "weight": 1}
	],
	"builtin_extenders": [{
		"name": "RackFilter",
		"conf": {
			"rackConstraints": {
				"10.175.106.128-26": 168,
				"10.175.106.192-26": 96
			},
			"rackSelector": {
				"10.175.106.128-26": {
					"subnet": "10.175.106.128-26"
				},
				"10.175.106.192-26": {
					"subnet": "10.175.106.192-26"
				}
			},
			"podSelector": {
				"network": "FLOATINGIP"
			}
		}
	}]
}
```

## 2. 自动从cmdb获取配置

扩容、缩容机器后需要重启apiswitch和kube-scheduler，apiswitch从k8s读取所有计算节点的ip后，会自动从cmdb拉取可以给容器用的ip，并生成kube-scheduler rackfilter插件的配置文件写入configmap

### 套件配置apiswitch参数

![image](doc/apiswitch-floatingip-from.png)

apiswitch.own.args增加 --floatingip-from=1

### 套件配置kube-scheduler从configmap读取rackfilter配置

![image](doc/scheduler-policy-file.png)

scheduler.own.args增加参数：

```
--policy-config-file=/etc/kubernetes/kube-scheduler-policy-config.json
```

![image](doc/scheduler-policy.png)

scheduler-policy-env-template配置框中填入以下配置

```
{
	"kind": "Policy",
	"apiVersion": "v1",
	"predicates": [
		{"name": "NoDiskConflict"}, 
		{"name": "NoVolumeZoneConflict"}, 
		{"name": "PodFitsHostPorts"}, 
		{"name": "PodFitsResources"}, 
		{"name": "PodFitsLocalDisk"}, 
		{"name": "HostName"}, 
		{"name": "MatchNodeSelector"}, 
		{"name": "PodToleratesNodeTaints"}, 
		{"name": "CheckNodeMemoryPressure"}, 
		{"name": "CheckNodeDiskPressure"}, 
		{"name": "MatchInterPodAffinity"}
	],
	"priorities": [
		{"name": "LeastRequestedPriority", "weight": 1}, 
		{"name": "BalancedResourceAllocation", "weight": 1}, 
		{"name": "ServiceSpreadingPriority", "weight": 1}, 
		{"name": "NodePreferAvoidPodsPriority", "weight": 10000}, 
		{"name": "NodeAffinityPriority", "weight": 1}, 
		{"name": "TaintTolerationPriority", "weight": 1}, 
		{"name": "InterPodAffinityPriority", "weight": 1}
	],
	"builtin_extenders": [{
		"name": "RackFilter",
		"conf": {
			"fromConfigMap": true,
			"configMapName": "floatingip-config",
			"configMapNamespace": "kube-system",
			"configMapDataKey": "scheduler-plugin-config",
			"podSelector": {
				"network": "FLOATINGIP"
			}
		}
	}]
}
```

### 自动生成的configmap示例

```
kubectl get cm -o json floatingip-config  -n kube-system                                                                                                            
```
