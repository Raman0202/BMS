# Deploy / Redeploy Runbook — bms-dev

Single container (MediaMTX + Go API via supervisord). Single node.
Image is loaded onto the node manually and the Deployment uses
`imagePullPolicy: Never`, so the cluster never pulls from ECR — no pull
secret, no `aws` CLI on the node needed.

Image: `475560691356.dkr.ecr.ap-south-1.amazonaws.com/bms/video/dev:latest`

---

## First deploy

### 1. Build the image (on the machine that has Docker + this repo)

From `prototype/`:
```bash
docker build -t 475560691356.dkr.ecr.ap-south-1.amazonaws.com/bms/video/dev:latest .
```

### 2. Save it to a file and copy to the node

```bash
docker save 475560691356.dkr.ecr.ap-south-1.amazonaws.com/bms/video/dev:latest -o bms.tar
scp bms.tar user@<node-ip>:/tmp/
```

If `docker save` produces something the node refuses to load (attestation /
manifest-list), build a plain single-arch tar instead:
```bash
docker buildx build --platform linux/amd64 --provenance=false \
  -t 475560691356.dkr.ecr.ap-south-1.amazonaws.com/bms/video/dev:latest \
  -o type=docker,dest=bms.tar .
```

### 3. Load it into the node's runtime (run ON the node)

Check the runtime first:
```bash
sudo crictl info 2>/dev/null | grep -i runtimeName || kubectl get node -o wide
```

Then load with the matching command:
```bash
# containerd (kubeadm / most clusters) — the k8s.io namespace is required
sudo ctr -n k8s.io images import /tmp/bms.tar

# k3s
sudo k3s ctr images import /tmp/bms.tar

# docker / cri-dockerd
sudo docker load -i /tmp/bms.tar
```

Confirm kubelet can see it:
```bash
sudo crictl images | grep bms
```

### 4. Apply the stack

```bash
kubectl apply -f portainer-stack.yaml     # or paste it into Portainer
kubectl -n bms-dev get pods -w
```

Verify:
```bash
curl http://<node-ip>:30080/api/fleet     # expect {"buses":[],...}
kubectl -n bms-dev logs deploy/bms-video         # both mediamtx + backend lines
```

---

## Redeploy a new image build

`:latest` never pulls (`imagePullPolicy: Never`), so you must reload the
tar onto the node, then roll the pod.

```bash
# 1. Rebuild + save + copy (steps 1–2 above)
# 2. Reload on the node
sudo ctr -n k8s.io images import /tmp/bms.tar    # or k3s / docker variant
# 3. Roll the pod so it picks up the new image
kubectl -n bms-dev rollout restart deploy/bms-video
kubectl -n bms-dev rollout status deploy/bms-video
```

---

## Config change only (mediamtx.yml)

Edit the ConfigMap block in `portainer-stack.yaml`, then:
```bash
kubectl apply -f portainer-stack.yaml
kubectl -n bms-dev rollout restart deploy/bms-video   # mounted config needs a pod restart
```

---

## Access

| Purpose | Address |
|---|---|
| JSON API (the one API) | `http://<node-ip>:30080/api/fleet` |
| RTMP publish (buses) | `rtmp://<node-ip>:31935/<bus_id>_<cam_no>` |
| WebRTC media | UDP `30189` on `bms-media.gna.energy` |

Node security group / firewall must allow inbound: `30080/TCP`, `31935/TCP`, `30189/UDP`.

## Preconditions to check once
- `kubectl get storageclass` — a default StorageClass must exist, else `recordings-pvc` stays Pending.
- DNS `A` record `bms-media.gna.energy` → node public IP (WebRTC ICE advertises this host).
- If pod shows `ErrImageNeverPull`: image name on the node ≠ manifest name, or it landed in the wrong containerd namespace (must be `k8s.io`). Recheck step 3.
