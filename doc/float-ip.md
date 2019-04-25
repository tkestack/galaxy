# Float IP

With the help of Galaxy-ipam, Galaxy offers Float IP for PODs of Kubernetes workloads. Float IP has a different meaning for each Release Policy and workload.

Galaxy currently supports Float IP function for Deployment and Statefulsets PODs.

## Usage

Specify extended resource requests and limits `tke.cloud.tencent.com/eni-ip:1` in one of the container definition of POD spec.

```
...
     containers:
     - image: nginx:1.7.9
       resources:
         limits:
           tke.cloud.tencent.com/eni-ip: "1"
         requests:
           tke.cloud.tencent.com/eni-ip: "1"
```

## Release Policy

Galaxy supports three kind of release policy. Add a POD annotation naming `k8s.v1.cni.galaxy.io/release-policy` with the following value:

- `never`, Never Release IP even if the Deployment or Statefulset is deleted. Submitting a same name Deployment or Statefulset will reuse previous reserved IPs. 
- `immutable`, Release IP Only when deleting or scaling down Deployment or Statefulset. If POD float onto a new node in case of original Node became NotReady, it will get the previous IP.

If the annotation is not specified or empty or any other value, the IP will be released once the POD floats or deleted.

## Float IP Pool

Galaxy also supports Deployment IP Pool which shares IPs among several Deployments by setting a `tke.cloud.tencent.com/eni-ip-pool` POD annotation with a given pool name as value.

Note that Float IP Pool Deployment is always `never` release policy regardless of the value of `k8s.v1.cni.galaxy.io/release-policy`.

### Pool size

By default, pool size grows as replicas of deployment or statefulset grows. While users can also specify a pool size to limit the ips allocated to
the pool which benefits upgrading deployments without changing IPs for blue-green release.

Creating a pool size can either by creating a pool CRD or by HTTP API. This is a example of creating pool size by CRD

```
apiVersion: galaxy.k8s.io/v1alpha1
kind: Pool
metadata:
  name: example-pool
size: 4
```

## API

Galaxy provides swagger 1.2 docs. Add `--swagger` command line args to galaxy-ipam and restart it, check `http://${galaxy-ipam-ip}:9041/apidocs.json/v1`
If you need a swagger 2.0 doc, please refer [api-spec-converter](https://github.com/LucyBot-Inc/api-spec-converter) to convert it.

API document below is converted by [swagger-markdown](https://github.com/syroegkin/swagger-markdown).

## Version: 1.0.0

### /v1/ip

#### GET
##### Summary:

List ips by keyword or params

##### Parameters

| Name | Located in | Description | Required | Schema |
| ---- | ---------- | ----------- | -------- | ---- |
| keyword | query | keyword | No | string |
| poolName | query | pool name | No | string |
| appName | query | app name | No | string |
| podName | query | pod name | No | string |
| namespace | query | namespace | No | string |
| isDeployment | query | listing deployments or statefulsets | No | boolean |
| page | query | page number, valid range [0,99999] | No | integer |
| size | query | page size, valid range (0,9999] | No | integer |
| sort | query | sort by which field, supports ip/namespace/podname/policy asc/desc | No | string |

##### Responses

| Code | Description | Schema |
| ---- | ----------- | ------ |
| 200 | request succeed | [api.ListIPResp](#api.listipresp) |

#### POST
##### Summary:

Release ips

##### Parameters

| Name | Located in | Description | Required | Schema |
| ---- | ---------- | ----------- | -------- | ---- |
| body | body |  | Yes | [api.ReleaseIPReq](#api.releaseipreq) |

##### Responses

| Code | Description | Schema |
| ---- | ----------- | ------ |
| 200 | request succeed | [api.ReleaseIPResp](#api.releaseipresp) |
| 202 | Unreleased ips have been released or allocated to other pods, or are not within valid range | [api.ReleaseIPResp](#api.releaseipresp) |
| 400 | 10.0.0.2 is not releasable |  |
| 500 | internal server error |  |

### /v1/pool

#### POST
##### Summary:

Create or update pool

##### Parameters

| Name | Located in | Description | Required | Schema |
| ---- | ---------- | ----------- | -------- | ---- |
| body | body |  | Yes | [api.Pool](#api.pool) |

##### Responses

| Code | Description | Schema |
| ---- | ----------- | ------ |
| 200 | request succeed | [httputil.Resp](#httputil.resp) |
| 400 | pool name is empty |  |
| 500 | internal server error |  |

### /v1/pool/{name}

#### DELETE
##### Summary:

Delete pool by name

##### Parameters

| Name | Located in | Description | Required | Schema |
| ---- | ---------- | ----------- | -------- | ---- |
| name | path | pool name | Yes | string |

##### Responses

| Code | Description | Schema |
| ---- | ----------- | ------ |
| 200 | request succeed | [httputil.Resp](#httputil.resp) |
| 400 | pool name is empty |  |
| 404 | pool not found |  |
| 500 | internal server error |  |

#### GET
##### Summary:

Get pool by name

##### Parameters

| Name | Located in | Description | Required | Schema |
| ---- | ---------- | ----------- | -------- | ---- |
| name | path | pool name | Yes | string |

##### Responses

| Code | Description | Schema |
| ---- | ----------- | ------ |
| 200 | request succeed | [api.GetPoolResp](#api.getpoolresp) |
| 400 | pool name is empty |  |
| 404 | pool not found |  |
| 500 | internal server error |  |

### Models


#### api.FloatingIP

| Name | Type | Description | Required |
| ---- | ---- | ----------- | -------- |
| appName | string | deployment or statefulset name | No |
| attr | string |  | Yes |
| ip | string | ip | Yes |
| isDeployment | boolean | deployment or statefulset | No |
| namespace | string | namespace | No |
| podName | string | pod name | No |
| policy | integer | ip release policy | Yes |
| poolName | string |  | No |
| releasable | boolean | if the ip is releasable. An ip is releasable if it isn't belong to any pod | No |
| status | string | pod status if exists | No |
| updateTime | dateTime | last allocate or release time of this ip | No |

#### api.GetPoolResp

| Name | Type | Description | Required |
| ---- | ---- | ----------- | -------- |
| code | integer |  | Yes |
| content | [httputil.Resp.content](#httputil.resp.content) |  | No |
| message | string |  | Yes |
| pool | [api.Pool](#api.pool) |  | Yes |

#### api.ListIPResp

| Name | Type | Description | Required |
| ---- | ---- | ----------- | -------- |
| content | [ [api.FloatingIP](#api.floatingip) ] |  | Yes |
| first | boolean | if this is the first page | Yes |
| last | boolean | if this is the last page | Yes |
| number | integer | page index starting from 0 | Yes |
| numberOfElements | integer | number of elements in this page | Yes |
| size | integer | page size | Yes |
| totalElements | integer | total number of elements | Yes |
| totalPages | integer | total number of pages | Yes |

#### api.Pool

| Name | Type | Description | Required |
| ---- | ---- | ----------- | -------- |
| name | string | pool name | Yes |
| size | integer | pool size | Yes |

#### api.ReleaseIPReq

| Name | Type | Description | Required |
| ---- | ---- | ----------- | -------- |
| ips | [ [api.FloatingIP](#api.floatingip) ] |  | Yes |

#### api.ReleaseIPResp

| Name | Type | Description | Required |
| ---- | ---- | ----------- | -------- |
| code | integer |  | Yes |
| content | [httputil.Resp.content](#httputil.resp.content) |  | No |
| message | string |  | Yes |
| unreleased | [ string ] | unreleased ips, have been released or allocated to other pods, or are not within valid range | No |

#### httputil.Resp

| Name | Type | Description | Required |
| ---- | ---- | ----------- | -------- |
| code | integer |  | Yes |
| content | [httputil.Resp.content](#httputil.resp.content) |  | No |
| message | string |  | Yes |

#### httputil.Resp.content

| Name | Type | Description | Required |
| ---- | ---- | ----------- | -------- |
| httputil.Resp.content |  |  |  |

#### page.Page.content

| Name | Type | Description | Required |
| ---- | ---- | ----------- | -------- |
| page.Page.content |  |  |  |
