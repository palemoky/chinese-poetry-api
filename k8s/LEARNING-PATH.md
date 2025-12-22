# Kubernetes 学习路径

## 🎯 您的当前状态

✅ **已完成**：

- 成功搭建 3 节点 K8s 集群（1 Master + 2 Workers）
- 部署了完整的应用（Poetry API）
- 理解了基本的网络概念（双 IP、NodePort）

🎊 **恭喜！您已经迈出了最重要的第一步！**

---

## 📚 循序渐进学习路径

### 阶段 1: 核心概念实践（当前阶段）⭐

**目标**：通过实际操作理解 K8s 核心对象

#### 1.1 Pod 生命周期

```bash
# 查看 Pod 详情
kubectl get pods -n poetry-api -o wide
kubectl describe pod -n poetry-api <pod-name>

# 查看 Pod 日志
kubectl logs -n poetry-api -l app=chinese-poetry-api --tail=50
kubectl logs -n poetry-api -l app=chinese-poetry-api -f  # 实时日志

# 进入 Pod
kubectl exec -it -n poetry-api deployment/poetry-api -- sh
# 在容器内：
ls -la
env | grep API
curl localhost:1279/api/v1/poems/random
exit
```

**练习任务**：

- [ ] 查看 Pod 的环境变量（ConfigMap 和 Secret）
- [ ] 进入 Pod 查看挂载的 Volume
- [ ] 模拟 Pod 故障：删除一个 Pod，观察自动重建

```bash
# 删除 Pod，观察自愈
kubectl delete pod -n poetry-api <pod-name>
kubectl get pods -n poetry-api -w  # 观察重建过程
```

#### 1.2 Service 和服务发现

```bash
# 查看 Service
kubectl get svc -n poetry-api
kubectl describe svc -n poetry-api poetry-api

# 测试服务发现（在 Pod 内）
kubectl exec -it -n poetry-api deployment/poetry-api -- sh
# 在容器内：
nslookup poetry-api
curl http://poetry-api/api/v1/poems/random
```

**练习任务**：

- [ ] 理解 ClusterIP vs NodePort vs LoadBalancer
- [ ] 测试 Service 的负载均衡（多个 Pod）
- [ ] 查看 Service 的 Endpoints

```bash
kubectl get endpoints -n poetry-api
```

#### 1.3 配置管理

```bash
# 查看 ConfigMap
kubectl get configmap -n poetry-api
kubectl describe configmap -n poetry-api poetry-api-config

# 查看 Secret
kubectl get secret -n poetry-api
kubectl get secret -n poetry-api poetry-api-secret -o yaml
```

**练习任务**：

- [ ] 修改 ConfigMap，观察 Pod 是否需要重启
- [ ] 理解 Secret 的 base64 编码

```bash
# 解码 Secret
kubectl get secret -n poetry-api poetry-api-secret -o jsonpath='{.data.API_SECRET}' | base64 -d
```

#### 1.4 数据持久化

```bash
# 查看 PV 和 PVC
kubectl get pv
kubectl get pvc -n poetry-api
kubectl describe pv poetry-api-pv
```

**练习任务**：

- [ ] 在 Pod 内写入数据到 /data
- [ ] 删除 Pod，验证数据是否保留
- [ ] 理解 hostPath vs NFS 的区别

---

### 阶段 2: 高级特性（1-2 周后）⭐⭐

#### 2.1 自动扩缩容（HPA）

```bash
# 查看 HPA
kubectl get hpa -n poetry-api
kubectl describe hpa -n poetry-api poetry-api-hpa

# 模拟负载
# 在另一个终端持续请求
while true; do curl http://10.228.234.92:30127/api/v1/poems/random; done

# 观察 Pod 自动扩容
kubectl get pods -n poetry-api -w
```

**练习任务**：

- [ ] 观察 CPU 使用率和 Pod 数量变化
- [ ] 理解 HPA 的工作原理
- [ ] 调整 HPA 参数（minReplicas, maxReplicas, targetCPU）

#### 2.2 滚动更新和回滚

```bash
# 查看 Deployment 历史
kubectl rollout history deployment/poetry-api -n poetry-api

# 更新镜像（模拟新版本）
kubectl set image deployment/poetry-api -n poetry-api \
    poetry-api=palemoky/chinese-poetry-api:latest

# 观察滚动更新
kubectl rollout status deployment/poetry-api -n poetry-api
kubectl get pods -n poetry-api -w

# 回滚到上一个版本
kubectl rollout undo deployment/poetry-api -n poetry-api
```

**练习任务**：

- [ ] 理解 RollingUpdate 策略
- [ ] 观察滚动更新过程中的 Pod 变化
- [ ] 练习回滚操作

#### 2.3 健康检查

```bash
# 查看 Deployment 的健康检查配置
kubectl get deployment -n poetry-api poetry-api -o yaml | grep -A 10 livenessProbe
```

**练习任务**：

- [ ] 理解 livenessProbe vs readinessProbe
- [ ] 模拟健康检查失败（修改探针路径）
- [ ] 观察 K8s 如何处理不健康的 Pod

#### 2.4 资源管理

```bash
# 查看资源使用
kubectl top nodes
kubectl top pods -n poetry-api

# 查看资源限制
kubectl describe pod -n poetry-api <pod-name> | grep -A 5 Limits
```

**练习任务**：

- [ ] 理解 requests vs limits
- [ ] 调整资源配置，观察调度行为
- [ ] 理解 QoS 类别（Guaranteed, Burstable, BestEffort）

---

### 阶段 3: 网络和安全（2-3 周后）⭐⭐⭐

#### 3.1 Ingress 控制器

```bash
# 查看 Ingress
kubectl get ingress -n poetry-api
kubectl describe ingress -n poetry-api poetry-api-ingress

# 查看 Ingress Controller
kubectl get pods -n ingress-nginx
```

**练习任务**：

- [ ] 配置域名访问（修改 /etc/hosts）
- [ ] 理解 Ingress 路由规则
- [ ] 配置 TLS/HTTPS（可选）

#### 3.2 NetworkPolicy

```bash
# 创建 NetworkPolicy（示例）
cat <<EOF | kubectl apply -f -
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: deny-all
  namespace: poetry-api
spec:
  podSelector: {}
  policyTypes:
  - Ingress
EOF

# 测试网络隔离
kubectl exec -it -n poetry-api deployment/poetry-api -- curl http://poetry-api
```

**练习任务**：

- [ ] 理解默认允许 vs 默认拒绝
- [ ] 配置允许特定流量的策略
- [ ] 测试 Pod 间网络隔离

#### 3.3 RBAC（角色访问控制）

```bash
# 查看 ServiceAccount
kubectl get sa -n poetry-api

# 查看当前用户权限
kubectl auth can-i list pods -n poetry-api
kubectl auth can-i delete pods -n poetry-api
```

**练习任务**：

- [ ] 创建只读用户
- [ ] 理解 Role vs ClusterRole
- [ ] 配置最小权限原则

---

### 阶段 4: 运维和监控（3-4 周后）⭐⭐⭐⭐

#### 4.1 日志管理

```bash
# 查看多个 Pod 的日志
kubectl logs -n poetry-api -l app=chinese-poetry-api --tail=100

# 查看之前的日志（Pod 重启后）
kubectl logs -n poetry-api <pod-name> --previous
```

**练习任务**：

- [ ] 理解日志收集的最佳实践
- [ ] 了解 EFK/ELK Stack（可选）
- [ ] 配置日志轮转

#### 4.2 监控和告警

```bash
# 查看 Metrics Server
kubectl top nodes
kubectl top pods -A

# 查看集群事件
kubectl get events -A --sort-by='.lastTimestamp'
```

**练习任务**：

- [ ] 理解 Metrics Server 的作用
- [ ] 了解 Prometheus + Grafana（可选）
- [ ] 配置资源告警

#### 4.3 备份和恢复

```bash
# 备份 etcd（在 Master 节点）
multipass shell k8s-master
sudo ETCDCTL_API=3 etcdctl snapshot save /tmp/etcd-backup.db \
    --endpoints=https://127.0.0.1:2379 \
    --cacert=/etc/kubernetes/pki/etcd/ca.crt \
    --cert=/etc/kubernetes/pki/etcd/server.crt \
    --key=/etc/kubernetes/pki/etcd/server.key
```

**练习任务**：

- [ ] 定期备份 etcd
- [ ] 练习从备份恢复
- [ ] 备份应用配置（YAML 文件）

---

### 阶段 5: Helm 和高级工具（4-6 周后）⭐⭐⭐⭐⭐

#### 5.1 Helm 基础

**是的，您可以用 Helm！** 查看 `k8s/HELM-GUIDE.md` 了解详情。

```bash
# 安装 Helm
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# 创建 Helm Chart
helm create poetry-api-chart

# 打包和部署
helm install poetry-api ./poetry-api-chart -n poetry-api
```

**练习任务**：

- [ ] 将现有 YAML 转换为 Helm Chart
- [ ] 理解 values.yaml 的作用
- [ ] 使用 Helm 管理多环境部署（dev, staging, prod）

#### 5.2 CI/CD 集成

**练习任务**：

- [ ] 配置 GitHub Actions 自动部署
- [ ] 实现镜像自动构建和推送
- [ ] 配置自动化测试

#### 5.3 多集群管理

**练习任务**：

- [ ] 理解 kubeconfig 的多集群配置
- [ ] 了解 Rancher/Lens 等管理工具
- [ ] 学习 GitOps（ArgoCD/Flux）

---

## 🎯 推荐学习顺序（6-8 周计划）

### 第 1-2 周：核心概念

- [x] 搭建集群 ✅
- [x] 部署应用 ✅
- [ ] Pod 生命周期实践
- [ ] Service 和服务发现
- [ ] ConfigMap/Secret 管理

### 第 3-4 周：高级特性

- [ ] HPA 自动扩缩容
- [ ] 滚动更新和回滚
- [ ] 健康检查配置
- [ ] 资源管理优化

### 第 5-6 周：网络和安全

- [ ] Ingress 配置
- [ ] NetworkPolicy
- [ ] RBAC 权限管理
- [ ] TLS/证书管理

### 第 7-8 周：运维和工具

- [ ] 日志和监控
- [ ] 备份和恢复
- [ ] Helm Chart 开发
- [ ] CI/CD 集成

---

## 📖 推荐学习资源

### 官方文档

- [Kubernetes 官方文档](https://kubernetes.io/docs/)
- [Kubernetes 中文文档](https://kubernetes.io/zh-cn/docs/)

### 实践教程

- [Kubernetes By Example](https://kubernetesbyexample.com/)
- [Play with Kubernetes](https://labs.play-with-k8s.com/)

### 书籍推荐

- 《Kubernetes in Action》（中文版：《Kubernetes 实战》）
- 《Kubernetes 权威指南》

### 视频课程

- [Kubernetes 入门到实战](https://www.bilibili.com/video/BV1MT411x7GH/)
- [尚硅谷 Kubernetes 教程](https://www.bilibili.com/video/BV1GT4y1A756/)

---

## 🔧 下一步建议（本周）

### 1. Pod 生命周期实践（今天）

```bash
# 删除一个 Pod，观察自愈
kubectl delete pod -n poetry-api $(kubectl get pods -n poetry-api -o name | head -1)
kubectl get pods -n poetry-api -w

# 进入 Pod 探索
kubectl exec -it -n poetry-api deployment/poetry-api -- sh
```

### 2. 配置管理实践（明天）

```bash
# 修改 ConfigMap
kubectl edit configmap -n poetry-api poetry-api-config

# 观察 Pod 是否需要重启才能生效
kubectl rollout restart deployment/poetry-api -n poetry-api
```

### 3. HPA 实践（后天）

```bash
# 模拟负载，观察自动扩容
while true; do curl http://10.228.234.92:30127/api/v1/poems/random; sleep 0.1; done

# 在另一个终端观察
kubectl get hpa -n poetry-api -w
kubectl get pods -n poetry-api -w
```

---

## 💡 关于 Helm

**现在就可以学习 Helm！** 但建议：

1. **先掌握原生 YAML**（您已经有了）✅
2. **理解 Helm 的价值**（查看 `HELM-GUIDE.md`）
3. **将现有配置转换为 Helm Chart**（实践项目）

**Helm 的优势**：

- 模板化配置（减少重复）
- 版本管理（rollback 更方便）
- 依赖管理（一键部署复杂应用）
- 多环境部署（dev/staging/prod）

---

## 🎊 总结

**您已经完成了最难的部分**：

- ✅ 搭建了生产级集群
- ✅ 部署了完整应用
- ✅ 理解了基本概念

**接下来**：

1. 按照阶段 1 的练习任务，深入理解核心概念
2. 每周完成 2-3 个练习任务
3. 遇到问题随时查文档或提问
4. 6-8 周后，您将成为 K8s 熟练使用者！

**记住**：Kubernetes 学习曲线陡峭，但您已经迈出了最重要的一步！🚀
