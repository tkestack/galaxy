[TOC]

# 确定机器是否在虚拟化区


虚拟化区能申请更多的IP。
输出“TYSV15108311 25 5.0E 通用区5”中带虚拟化区字样的就是虚拟化区的机器，5.0E表示网络架构，25表示机器的网段剩余IP量。
虚拟化区剩余IP值没有参考意义，因为虚拟化区预留给VM的IP段跟主机段不一样
```
http://sniper.oa.com/iprms_v2/request/check_ip_available_view
a="TYSV11034407-1 TYSV11034407-2 TYSV11034407-3 TYSV11034407-4 TYSV110902A2-2 TYSV110902A5-4 TYSV110902AF-1 TYSV110902AF-2 TYSV110902AF-3 TYSV110902C1-4"
a.split(" ").forEach(function(e){$.ajax({async: false, type: "POST", url: "check_ip_available", contentType: 'application/json', data: JSON.stringify({"AssetId": e, "RequestType": "0"}), success: function(r) {var obj = jQuery.parseJSON(r); var total=0; for (var i=0; i<obj.data.available_segment.length; i++) {total+=obj.data.available_segment[i].UnusedIPCount;} console.log(e + " " +total+" "+obj.data.DmnStructureVersionName+" "+obj.data.DmnName);}})})

TYSV15108311 25 5.0E 通用区5
TYSV15108219 25 V3.5E 通用虚拟化区3
TYSV15108220 25 V3.5E 通用虚拟化区3
TYSV15108222 25 V3.5E 通用虚拟化区3
TYSV15108223 25 V3.5E 通用虚拟化区3
TYSV15108224 25 V3.5E 通用虚拟化区3
TYSV15108225 25 V3.5E 通用虚拟化区3
TYSV1510AE3S 25 V3.5E 通用虚拟化区3
TYSV1510AE46 28 V3.5E 通用虚拟化区3
TYSV1510AE47 28 V3.5E 通用虚拟化区3
TYSV15108226 28 V3.5E 通用虚拟化区3
TYSV1510AE42 28 V3.5E 通用虚拟化区3
TYSV1510AE3E 25 V3.5E 通用虚拟化区3
TYSV1308261F-1 11 V3.5.3 通用区1
TYSV14100294 15 V3.5E 通用区3
TYSV141002K3 15 V3.5E 通用区3
TYSV1410026A 15 V3.5E 通用区3
TYSV1410026K 15 V3.5E 通用区3
TYSV15108251 32 5.0E 通用虚拟化区4
TYSV1510830H 17 5.0E 通用区5
TYSV1510830K 17 5.0E 通用区5
TYSV1510831K 17 5.0E 通用区5
TYSV15108310 17 5.0E 通用区5
TYSV15108311 25 5.0E 通用区5
```

# 虚拟化区机器按虚拟化需求创建一定量的VM固资编号

提前跟运维或者业务方确定VM固资编号要放在哪个业务集合下（busi1_name, busi2_name, busi3_name)，负责人和备份负责人（operator，bakoperator）

**修改脚本中的固资编号，修改$(seq 10)为想要的虚拟化比**，生成的VM固资编号在vms文件中

systemId和sceneId是我们在cmdb系统中创建的接口调用方的标识，不需要修改。
生成固资编号的接口是有调用IP白名单的，我们申请的调用方标识（docker_on_gaia系统）在这个链接可以查到 http://tako.oa.com/?app_id=54&app_type=app&suburl=page=/helper/help_tools

可以选择性修改docker_on_gaia系统ip地址，加入准备调用接口的机器的ip，然后在那些机器上执行下面的脚本（如果发现访问172.16.8.71 80端口网络不通，前往 http://www.itil.com/ 提IDC内网访问控制 紧急需求，开通策略。）
```
for i in TYSV15108219 TYSV15108220 TYSV15108251; do for j in $(seq 10); do curl -s -H "Content-Type: application/json; charset=UTF-8" -X POST -d '{"params":{"content":{"schemeId":"server_vm","type":"Json","version":"1.0","requestInfo":{"systemId":"201503250","sceneId":"3777","requestModule":"","operator":"ramichen"},"actions":[{"add_vm":{"data":{"assetid":"'$i'","operator":"ramichen","bakoperator":"werwang;dockerzhang","dept_id":9,"dept_name":"数据平台部","group_id":1,"group_name":"未分配小组","vm_type":"VSELF","busi1_name":"数据仓库","busi2_name":"盖娅","busi3_name":"	gaiaStack-[现网][Kube_Kubelet][主][szps-public]","alarmlevel":"L1"},"condition":{},"reason":"add new vm"}}]}}}' http://172.16.8.71/api/modify/modify | python -c "import json,sys;obj=json.load(sys.stdin);print obj['dataSet']['result'][0]['add_vm']['key'];" >> vms; done; done
```

注：

group_name, group_id, dept_id, dept_name, busi1_name, busi2_name, busi3_name 在下面的CMDB页面中可查询到。
接口选择server（服务器）,
参数数据中选择：运维部门ID(DeptId) 运维部门(DeptName) 运维小组ID(GroupId) 运维小组(GroupName) 业务集合(serverBusi1) 业务(serverBusi2) 业务模块(BsiName),
查询条件选择：固资编号(SvrAssetId)
http://tako.oa.com/?app_id=54&app_type=app&suburl=page=/helper/help_tools

# 申请IP

非虚拟化区只能用主机固资编号申请IP，虚拟化区用VM固资申请的IP是预留给虚拟机的IP段，用主机固资申请的是预留给机器的IP段。
所以非虚拟化区用主机固资编号申请IP，虚拟化区用VM固资申请

申请IP链接 http://sniper.oa.com/iprms_v2/request/allot ，记得选择内网IP，修改申请IP个数

# 迁移IP

这一步可选，如果为了便于管理，非虚拟化区申请好的IP可以迁移到VM固资上。
按上面创建固资的方法给非虚拟化区的机器也创建一批VM固资，然后调用下面的接口将IP迁移到VM固资编号上，**注意不要把原主机IP也迁移了**

http://sniper.oa.com/iprms_v2/request/shift 

```
$.ajax({type: 'POST',contentType: 'application/json',url:'req_shift_ip',data:JSON.stringify([{FromAssetId: "TYSV12091072", ToAssetId: "TYSV12091072-VM3898", IpList: "10.191.134.169"},{FromAssetId: "TYSV1209103H", ToAssetId: "TYSV1209103H-VM3901", IpList: "10.191.135.46"},{FromAssetId: "TYSV1209103E", ToAssetId: "TYSV1209103E-VM3904", IpList: "10.191.135.56"}]),success: function(data){console.log(data);}})
```

FromAssetId:主机固资编号 ToAssetId:VM固资编号 IpList:迁移IP

# 删除VM固资编号

**注意先回收这些固资的IP** http://sniper.oa.com/iprms_v2/request/callback
```
for i in TYSV15108219-VM0117 TYSV15108219-VM0120; do echo $i; curl -H "Content-Type: application/json; charset=UTF-8" -X POST -d '{"params":{"content":{"schemeId":"server_vm","type":"Json","version":"1.0","requestInfo":{"systemId":"201503250","sceneId":"3795","requestModule":"","operator":"ramichen"},"actions":[{"delete_vm":{"data":{},"condition":{"assetid":"'`printf $i`'"},"reason":"delete"}}]}}}' http://172.16.8.71/api/modify/modify; done
```

# 忘记了主机申请了哪些VM固资编号

在 http://config2.itil.com/server/server 根据虚拟母机固资编号可以查询到VM固资编号
