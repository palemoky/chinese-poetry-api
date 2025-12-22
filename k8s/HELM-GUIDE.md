# Helm vs 原生 K8s YAML 对比

## 📊 问题：配置文件太多

### 当前项目（原生 YAML）

```
k8s/
├── namespace.yaml           # 命名空间
├── configmap.yaml           # 配置
├── secret.yaml              # 密钥
├── persistent-volume.yaml   # 存储
├── deployment.yaml          # 部署
├── service.yaml             # 服务
├── hpa.yaml                 # 自动扩缩容
├── ingress.yaml             # 外部访问
├── job.yaml                 # 任务
├── cronjob.yaml             # 定时任务
└── statefulset.yaml         # 有状态部署

总共：11 个文件
```

**部署步骤**：

```bash
kubectl apply -f namespace.yaml
kubectl apply -f configmap.yaml
kubectl apply -f secret.yaml
kubectl apply -f persistent-volume.yaml
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml
kubectl apply -f hpa.yaml
kubectl apply -f ingress.yaml
# ... 😫 需要记住顺序和依赖关系
```

---

## 🎯 Helm 的解决方案

### Helm Chart 结构

```
poetry-api-chart/
├── Chart.yaml              # Chart 元数据
├── values.yaml             # 默认配置参数
├── values-dev.yaml         # 开发环境配置
├── values-prod.yaml        # 生产环境配置
└── templates/              # 模板目录
    ├── namespace.yaml
    ├── configmap.yaml
    ├── secret.yaml
    ├── deployment.yaml
    ├── service.yaml
    ├── hpa.yaml
    └── ingress.yaml
```

### Chart.yaml（元数据）

```yaml
apiVersion: v2
name: poetry-api
description: Chinese Poetry API Helm Chart
type: application
version: 1.0.0
appVersion: "1.0.0"
```

### values.yaml（配置参数）

```yaml
# 副本数
replicaCount: 2

# 镜像配置
image:
  repository: palemoky/chinese-poetry-api
  tag: latest
  pullPolicy: IfNotPresent

# 服务配置
service:
  type: ClusterIP
  port: 1279
  nodePort: 30127

# Ingress 配置
ingress:
  enabled: true
  host: poetry-api.local

# 资源限制
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 512Mi

# HPA 配置
autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilizationPercentage: 70

# 存储配置
persistence:
  enabled: true
  size: 5Gi
  storageClass: manual
```

### templates/deployment.yaml（模板）

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: { { .Chart.Name } }
  namespace: { { .Values.namespace } }
spec:
  replicas: { { .Values.replicaCount } } # 参数化
  selector:
    matchLabels:
      app: { { .Chart.Name } }
  template:
    metadata:
      labels:
        app: { { .Chart.Name } }
    spec:
      containers:
        - name: { { .Chart.Name } }
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          imagePullPolicy: { { .Values.image.pullPolicy } }
          ports:
            - containerPort: { { .Values.service.port } }
          resources: { { - toYaml .Values.resources | nindent 12 } } # 参数化
```

---

## 🚀 使用对比

### 原生 K8s YAML

```bash
# 部署到开发环境
kubectl apply -f namespace.yaml
kubectl apply -f configmap.yaml
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml
# ... 需要手动修改每个文件的参数

# 部署到生产环境
# 需要复制一套文件，修改所有参数
# 或者使用 kustomize
```

### Helm Chart

```bash
# 部署到开发环境（1 个副本）
helm install poetry-api ./poetry-api-chart \
  --values values-dev.yaml \
  --set replicaCount=1

# 部署到生产环境（5 个副本）
helm install poetry-api ./poetry-api-chart \
  --values values-prod.yaml \
  --set replicaCount=5

# 升级
helm upgrade poetry-api ./poetry-api-chart \
  --set image.tag=v2.0

# 回滚
helm rollback poetry-api 1

# 卸载
helm uninstall poetry-api
```

---

## 📈 Helm 的核心优势

### 1. **参数化配置**

**问题**：不同环境需要不同配置

```yaml
# 开发环境：1 个副本，小资源
# 测试环境：2 个副本，中资源
# 生产环境：5 个副本，大资源
```

**Helm 解决**：

```bash
# values-dev.yaml
replicaCount: 1
resources:
  limits:
    cpu: 200m
    memory: 256Mi

# values-prod.yaml
replicaCount: 5
resources:
  limits:
    cpu: 1000m
    memory: 2Gi

# 使用
helm install app ./chart -f values-dev.yaml
helm install app ./chart -f values-prod.yaml
```

### 2. **版本管理和回滚**

```bash
# 查看历史版本
helm history poetry-api
# REVISION  STATUS      CHART           APP VERSION
# 1         superseded  poetry-api-1.0  1.0.0
# 2         superseded  poetry-api-1.1  1.1.0
# 3         deployed    poetry-api-1.2  1.2.0

# 回滚到版本 1
helm rollback poetry-api 1
```

### 3. **依赖管理**

```yaml
# Chart.yaml
dependencies:
  - name: postgresql
    version: 12.1.0
    repository: https://charts.bitnami.com/bitnami
  - name: redis
    version: 17.3.0
    repository: https://charts.bitnami.com/bitnami

# 一条命令安装所有依赖
helm dependency update
helm install poetry-api ./chart
```

### 4. **打包和分发**

```bash
# 打包
helm package poetry-api-chart/
# 生成: poetry-api-1.0.0.tgz

# 上传到 Chart 仓库
helm repo add myrepo https://charts.example.com
helm push poetry-api-1.0.0.tgz myrepo

# 其他人使用
helm repo add myrepo https://charts.example.com
helm install my-app myrepo/poetry-api
```

---

## 🆚 完整对比

| 特性             | 原生 K8s YAML          | Helm Chart                  |
| ---------------- | ---------------------- | --------------------------- |
| **配置文件数量** | 10+ 个独立文件         | 1 个 Chart 包               |
| **部署命令**     | `kubectl apply -f` × N | `helm install` × 1          |
| **参数修改**     | 编辑每个 YAML 文件     | 修改 values.yaml 或 `--set` |
| **多环境部署**   | 复制文件或 kustomize   | 不同 values 文件            |
| **版本管理**     | 手动 Git 管理          | 内置版本控制                |
| **回滚**         | 手动 `kubectl apply`   | `helm rollback`             |
| **依赖管理**     | 手动安装依赖           | 自动处理依赖                |
| **打包分发**     | 压缩文件               | Helm 仓库                   |
| **学习曲线**     | 简单                   | 中等                        |
| **适用场景**     | 简单应用、学习         | 复杂应用、生产环境          |

---

## 💡 何时使用 Helm？

### ✅ 适合使用 Helm

- 多环境部署（dev、test、prod）
- 需要频繁更新和回滚
- 有多个依赖服务
- 团队协作，需要标准化
- 需要打包分发

### ❌ 不需要 Helm

- 简单的单一应用
- 学习 K8s 基础概念
- 配置很少变化
- 只有一个环境

---

## 🎯 学习路径建议

### 阶段 1: 原生 YAML（当前）

**目的**：理解 K8s 核心概念

```bash
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml
```

**优势**：

- 直观理解每个资源
- 掌握 K8s 基础

### 阶段 2: Kustomize（中级）

**目的**：管理多环境配置

```bash
# base/
kustomization.yaml
deployment.yaml

# overlays/dev/
kustomization.yaml  # 覆盖 base

# overlays/prod/
kustomization.yaml  # 覆盖 base
```

### 阶段 3: Helm（高级）

**目的**：企业级应用管理

```bash
helm install app ./chart -f values-prod.yaml
```

---

## 📚 总结

**您的理解完全正确！**

1. ✅ K8s 配置文件多且复杂
2. ✅ Helm 很好地解决了这个问题
3. ✅ Helm = K8s 的包管理器

**类比**：

- K8s YAML = 手动编译安装软件
- Helm = apt/yum 包管理器

**建议**：

- 先掌握原生 YAML（理解基础）
- 再学习 Helm（提高效率）
- 生产环境优先使用 Helm
