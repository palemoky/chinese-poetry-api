# 生产环境最佳实践

本文档介绍如何将学习环境的配置应用到生产环境，以及生产环境的最佳实践。

## 📋 目录

- [生产环境与学习环境的区别](#生产环境与学习环境的区别)
- [高可用架构](#高可用架构)
- [安全加固](#安全加固)
- [资源管理](#资源管理)
- [监控和日志](#监控和日志)
- [备份和恢复](#备份和恢复)
- [CI/CD 集成](#cicd-集成)

## 🔄 生产环境与学习环境的区别

### 学习环境（当前配置）

- **节点数量**: 1 Master + 2 Worker
- **高可用**: 无
- **存储**: 本地 hostPath
- **网络**: 单网卡
- **监控**: 基础 Metrics Server
- **备份**: 手动

### 生产环境建议

- **节点数量**: 3 Master + 3+ Worker（高可用）
- **高可用**: etcd 集群、多 Master
- **存储**: 分布式存储（Ceph、NFS）
- **网络**: 多网卡、网络策略
- **监控**: Prometheus + Grafana
- **备份**: 自动化备份

## 🏗️ 高可用架构

### 多 Master 节点

生产环境应使用至少 3 个 Master 节点：

```bash
# 创建 3 个 Master 节点
for i in 1 2 3; do
  multipass launch --name k8s-master-$i \
    --cpus 4 --memory 8G --disk 40G
done

# 使用 HAProxy 或 Nginx 作为负载均衡器
# 初始化第一个 Master
kubeadm init --control-plane-endpoint "LOAD_BALANCER_IP:6443" \
  --upload-certs \
  --pod-network-cidr=10.244.0.0/16

# 加入其他 Master 节点
kubeadm join LOAD_BALANCER_IP:6443 \
  --token <token> \
  --discovery-token-ca-cert-hash sha256:<hash> \
  --control-plane \
  --certificate-key <cert-key>
```

### etcd 高可用

- 使用奇数个 etcd 节点（3、5、7）
- 定期备份 etcd 数据
- 监控 etcd 性能

```bash
# 备份 etcd
ETCDCTL_API=3 etcdctl snapshot save /backup/etcd-snapshot.db \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key

# 恢复 etcd
ETCDCTL_API=3 etcdctl snapshot restore /backup/etcd-snapshot.db
```

## 🔒 安全加固

### 1. RBAC 权限控制

```yaml
# 创建只读用户
apiVersion: v1
kind: ServiceAccount
metadata:
  name: readonly-user
  namespace: poetry-api

---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: readonly-role
  namespace: poetry-api
rules:
  - apiGroups: [""]
    resources: ["pods", "services"]
    verbs: ["get", "list", "watch"]

---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: readonly-binding
  namespace: poetry-api
subjects:
  - kind: ServiceAccount
    name: readonly-user
roleRef:
  kind: Role
  name: readonly-role
  apiGroup: rbac.authorization.k8s.io
```

### 2. Pod Security Standards

```yaml
# 启用 Pod Security Admission
apiVersion: v1
kind: Namespace
metadata:
  name: poetry-api
  labels:
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/audit: restricted
    pod-security.kubernetes.io/warn: restricted
```

### 3. Network Policy

```yaml
# 限制 Pod 网络访问
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: poetry-api-netpol
  namespace: poetry-api
spec:
  podSelector:
    matchLabels:
      app: chinese-poetry-api
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              name: ingress-nginx
      ports:
        - protocol: TCP
          port: 1279
  egress:
    - to:
        - namespaceSelector: {}
      ports:
        - protocol: TCP
          port: 53 # DNS
    - to:
        - podSelector: {}
```

### 4. Secret 加密

```bash
# 启用 Secret 加密
cat <<EOF > /etc/kubernetes/enc/enc.yaml
apiVersion: apiserver.config.k8s.io/v1
kind: EncryptionConfiguration
resources:
  - resources:
      - secrets
    providers:
      - aescbc:
          keys:
            - name: key1
              secret: <base64-encoded-secret>
      - identity: {}
EOF

# 在 kube-apiserver 中启用
--encryption-provider-config=/etc/kubernetes/enc/enc.yaml
```

### 5. 镜像安全

```yaml
# 使用私有镜像仓库
apiVersion: v1
kind: Secret
metadata:
  name: regcred
  namespace: poetry-api
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: <base64-encoded-docker-config>

---
# 在 Deployment 中使用
spec:
  template:
    spec:
      imagePullSecrets:
        - name: regcred
      containers:
        - name: poetry-api
          image: your-registry.com/poetry-api:latest
          imagePullPolicy: Always
```

## 💾 资源管理

### 1. Resource Quotas

```yaml
# 限制 Namespace 资源使用
apiVersion: v1
kind: ResourceQuota
metadata:
  name: poetry-api-quota
  namespace: poetry-api
spec:
  hard:
    requests.cpu: "10"
    requests.memory: 20Gi
    limits.cpu: "20"
    limits.memory: 40Gi
    persistentvolumeclaims: "5"
    services.loadbalancers: "2"
```

### 2. LimitRange

```yaml
# 设置默认资源限制
apiVersion: v1
kind: LimitRange
metadata:
  name: poetry-api-limits
  namespace: poetry-api
spec:
  limits:
    - max:
        cpu: "2"
        memory: 2Gi
      min:
        cpu: 100m
        memory: 128Mi
      default:
        cpu: 500m
        memory: 512Mi
      defaultRequest:
        cpu: 200m
        memory: 256Mi
      type: Container
```

### 3. PodDisruptionBudget

```yaml
# 确保最小可用 Pod 数量
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: poetry-api-pdb
  namespace: poetry-api
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app: chinese-poetry-api
```

## 📊 监控和日志

### 1. Prometheus + Grafana

```bash
# 安装 Prometheus Operator
kubectl create namespace monitoring

helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm install prometheus prometheus-community/kube-prometheus-stack \
  --namespace monitoring

# 访问 Grafana
kubectl port-forward -n monitoring svc/prometheus-grafana 3000:80
```

### 2. 应用监控

```yaml
# 添加 Prometheus 注解
apiVersion: v1
kind: Service
metadata:
  name: poetry-api
  namespace: poetry-api
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "1279"
    prometheus.io/path: "/metrics"
```

### 3. 日志收集

```bash
# 安装 EFK Stack (Elasticsearch + Fluentd + Kibana)
kubectl create namespace logging

# 或使用 Loki
helm repo add grafana https://grafana.github.io/helm-charts
helm install loki grafana/loki-stack \
  --namespace logging \
  --set grafana.enabled=true
```

### 4. 告警规则

```yaml
# Prometheus 告警规则
apiVersion: v1
kind: ConfigMap
metadata:
  name: prometheus-rules
  namespace: monitoring
data:
  poetry-api.rules: |
    groups:
      - name: poetry-api
        rules:
          - alert: PodDown
            expr: up{job="poetry-api"} == 0
            for: 5m
            labels:
              severity: critical
            annotations:
              summary: "Pod is down"

          - alert: HighMemoryUsage
            expr: container_memory_usage_bytes{pod=~"poetry-api.*"} / container_spec_memory_limit_bytes > 0.9
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "High memory usage"
```

## 💾 备份和恢复

### 1. etcd 备份

```bash
# 创建定时备份脚本
cat <<'EOF' > /usr/local/bin/backup-etcd.sh
#!/bin/bash
BACKUP_DIR="/backup/etcd"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)

mkdir -p "$BACKUP_DIR"

ETCDCTL_API=3 etcdctl snapshot save \
  "$BACKUP_DIR/etcd-snapshot-$TIMESTAMP.db" \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key

# 保留最近 7 天的备份
find "$BACKUP_DIR" -name "etcd-snapshot-*.db" -mtime +7 -delete
EOF

chmod +x /usr/local/bin/backup-etcd.sh

# 添加 cron 任务
echo "0 2 * * * /usr/local/bin/backup-etcd.sh" | crontab -
```

### 2. 应用数据备份

```yaml
# 使用 Velero 备份整个集群
apiVersion: velero.io/v1
kind: Backup
metadata:
  name: poetry-api-backup
  namespace: velero
spec:
  includedNamespaces:
    - poetry-api
  storageLocation: default
  volumeSnapshotLocations:
    - default
  ttl: 720h # 30 天
```

### 3. PV 备份

```bash
# 使用 rsync 备份 PV 数据
rsync -avz /mnt/data/poetry-api/ /backup/pv-data/
```

## 🚀 CI/CD 集成

### 1. GitOps with ArgoCD

```bash
# 安装 ArgoCD
kubectl create namespace argocd
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

# 创建 Application
cat <<EOF | kubectl apply -f -
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: poetry-api
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/palemoky/chinese-poetry-api
    targetRevision: main
    path: k8s
  destination:
    server: https://kubernetes.default.svc
    namespace: poetry-api
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
EOF
```

### 2. GitHub Actions 集成

```yaml
# .github/workflows/deploy-k8s.yml
name: Deploy to Kubernetes

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Set up kubectl
        uses: azure/setup-kubectl@v3

      - name: Configure kubectl
        run: |
          echo "${{ secrets.KUBECONFIG }}" > kubeconfig
          export KUBECONFIG=./kubeconfig

      - name: Deploy
        run: |
          kubectl apply -k k8s/
          kubectl rollout status deployment/poetry-api -n poetry-api
```

## 📝 生产环境检查清单

### 部署前

- [ ] 高可用配置（3+ Master 节点）
- [ ] 网络策略配置
- [ ] RBAC 权限配置
- [ ] Secret 加密启用
- [ ] 资源配额设置
- [ ] 镜像扫描通过
- [ ] 备份策略配置

### 部署后

- [ ] 监控告警配置
- [ ] 日志收集配置
- [ ] 备份验证
- [ ] 灾难恢复演练
- [ ] 性能测试
- [ ] 安全扫描
- [ ] 文档更新

## 🔗 相关资源

- [Kubernetes Production Best Practices](https://kubernetes.io/docs/setup/best-practices/)
- [CIS Kubernetes Benchmark](https://www.cisecurity.org/benchmark/kubernetes)
- [CNCF Cloud Native Security](https://www.cncf.io/projects/security/)

## 💡 总结

生产环境需要考虑：

1. **高可用**: 多 Master、etcd 集群
2. **安全**: RBAC、NetworkPolicy、Secret 加密
3. **监控**: Prometheus、日志收集、告警
4. **备份**: 自动化备份、灾难恢复
5. **自动化**: GitOps、CI/CD

从学习环境到生产环境是一个渐进的过程，建议先在测试环境验证所有配置。
