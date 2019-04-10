# Running Galaxy

Please refer to [building](building.md) for building docker image first.

Copy all files in yaml directory to you kubectl client machine.
And then docker push your image to Docker hub or private registry server.

```
# Please update image prefix in these yamls
kubectl create -f galaxy.yaml
```

# Starting a deployment

```
cat <<EOF | kubectl create -f -
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
 labels:
   app: nginx
 name: nginx
spec:
 replicas: 1
 selector:
   matchLabels:
     app: nginx
 template:
   metadata:
     labels:
       app: nginx
   spec:
     containers:
     - image: nginx:1.7.9
       name: nginx
       resources:
         limits:
           tke.cloud.tencent.com/eni-ip: "1"
         requests:
           tke.cloud.tencent.com/eni-ip: "1"
EOF
```
