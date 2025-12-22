# Multipass + kubeadm 生产级 K8s 集群

使用 Multipass 和 kubeadm 在 Ubuntu 24.04 上搭建生产级 Kubernetes 集群。

## 📋 目录

- [系统要求](#系统要求)
- [集群架构](#集群架构)
- [快速开始](#快速开始)
- [详细步骤](#详细步骤)
- [集群管理](#集群管理)
- [部署应用](#部署应用)
- [故障排查](#故障排查)

## 💻 系统要求

### 硬件要求

- **CPU**: AMD 3900X（12 核 24 线程）
- **内存**: 32GB（集群使用 24GB，留 8GB 给宿主机）
- **磁盘**: 至少 100GB 可用空间
- **系统**: Ubuntu 24.04 Server

### 软件要求

- Multipass（通过 snap 安装）
- 支持 KVM 虚拟化

## 🏗️ 集群架构

### 节点配置

| 节点        | 角色   | CPU | 内存 | 磁盘 |
| ----------- | ------ | --- | ---- | ---- |
| k8s-master  | Master | 4   | 8GB  | 40GB |
| k8s-worker1 | Worker | 4   | 8GB  | 40GB |
| k8s-worker2 | Worker | 4   | 8GB  | 40GB |

### 组件版本

- **Kubernetes**: 1.35（最新版本，2025-12-17 发布）
- **Container Runtime**: containerd 2.0+
- **Network Plugin**: Calico 3.29
- **Ingress Controller**: Nginx Ingress 1.12
- **Load Balancer**: MetalLB 0.14
- **Metrics**: Metrics Server

> [!IMPORTANT]
> Kubernetes 1.35 要求：
>
> - **cgroup v2**（Ubuntu 24.04 默认支持 ✅）
> - **containerd 2.0+**（脚本会自动检查）

## 🚀 快速开始

### 1. 安装 Multipass

```bash
# 安装 Multipass
sudo snap install multipass

# 验证安装
multipass version
```

### 2. 一键创建集群

```bash
# 进入 multipass 目录
cd k8s/multipass

# 运行安装脚本（约 10-15 分钟）
./setup-cluster.sh
```

脚本会自动完成：

1. ✅ 创建 3 个虚拟机
2. ✅ 安装 Kubernetes 组件
3. ✅ 初始化 Master 节点
4. ✅ 加入 Worker 节点
5. ✅ 安装 Calico 网络插件
6. ✅ 安装 Metrics Server
7. ✅ 安装 Nginx Ingress Controller
8. ✅ 安装 MetalLB 负载均衡器

### 3. 验证集群

```bash
# 设置 kubeconfig
export KUBECONFIG=$(pwd)/kubeconfig

# 查看节点
kubectl get nodes

# 查看所有 Pods
kubectl get pods -A
```

## 📖 详细步骤

### 步骤 1: 创建虚拟机

脚本会创建 3 个 Ubuntu 虚拟机：

```bash
multipass list
```

输出示例：

```
Name            State    IPv4           Image
k8s-master      Running  192.168.64.2   Ubuntu 24.04 LTS
k8s-worker1     Running  192.168.64.3   Ubuntu 24.04 LTS
k8s-worker2     Running  192.168.64.4   Ubuntu 24.04 LTS
```

### 步骤 2: 安装 Kubernetes 组件

每个虚拟机会安装：

- containerd（容器运行时）
- kubelet（节点代理）
- kubeadm（集群管理工具）
- kubectl（命令行工具）

### 步骤 3: 初始化 Master 节点

Master 节点会初始化：

- API Server
- Controller Manager
- Scheduler
- etcd

### 步骤 4: 加入 Worker 节点

Worker 节点会自动加入集群并开始运行工作负载。

### 步骤 5: 安装网络插件

Calico 提供：

- Pod 网络
- NetworkPolicy 支持
- IPAM（IP 地址管理）

### 步骤 6: 安装附加组件

- **Metrics Server**: 提供资源监控（CPU、内存）
- **Nginx Ingress**: HTTP/HTTPS 路由
- **MetalLB**: LoadBalancer 服务支持

## 🔧 集群管理

### 管理脚本

使用 `manage-cluster.sh` 管理集群：

```bash
# 查看帮助
./manage-cluster.sh help

# 查看集群状态
./manage-cluster.sh status

# 查看集群信息
./manage-cluster.sh info

# 停止集群
./manage-cluster.sh stop

# 启动集群
./manage-cluster.sh start

# 重启集群
./manage-cluster.sh restart

# 删除集群
./manage-cluster.sh delete

# 进入 Master 节点
./manage-cluster.sh shell

# 查看节点日志
./manage-cluster.sh logs
```

### 手动管理虚拟机

```bash
# 查看虚拟机列表
multipass list

# 进入虚拟机
multipass shell k8s-master

# 停止虚拟机
multipass stop k8s-master

# 启动虚拟机
multipass start k8s-master

# 删除虚拟机
multipass delete k8s-master
multipass purge
```

## 📦 部署应用

### 一键部署 Poetry API

```bash
# 部署应用
./deploy-app.sh
```

脚本会自动：

1. 创建数据目录
2. 应用 K8s 配置
3. 等待 Pods 就绪
4. 显示访问信息

### 手动部署

```bash
# 设置 kubeconfig
export KUBECONFIG=$(pwd)/kubeconfig

# 创建数据目录
for node in k8s-master k8s-worker1 k8s-worker2; do
  multipass exec "$node" -- sudo mkdir -p /mnt/data/poetry-api
  multipass exec "$node" -- sudo chmod 777 /mnt/data/poetry-api
done

# 部署应用
kubectl apply -k ../

# 查看部署状态
kubectl get all -n poetry-api
```

### 访问应用

#### 方式 1: NodePort

```bash
# 获取 Master IP 和 NodePort
MASTER_IP=$(multipass info k8s-master | grep IPv4 | awk '{print $2}')
NODE_PORT=$(kubectl get svc -n poetry-api poetry-api-nodeport -o jsonpath='{.spec.ports[0].nodePort}')

# 访问 API
curl http://${MASTER_IP}:${NODE_PORT}/api/v1/poems/random
```

#### 方式 2: LoadBalancer（MetalLB）

```bash
# 获取 LoadBalancer IP
LB_IP=$(kubectl get svc -n poetry-api poetry-api -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

# 访问 API
curl http://${LB_IP}/api/v1/poems/random
```

#### 方式 3: Ingress

```bash
# 配置 hosts
echo "${MASTER_IP} poetry-api.local" | sudo tee -a /etc/hosts

# 访问 API
curl http://poetry-api.local/api/v1/poems/random
```

## 🔍 故障排查

### 虚拟机问题

#### 虚拟机无法启动

```bash
# 查看虚拟机状态
multipass list

# 查看虚拟机详情
multipass info k8s-master

# 重启 Multipass 服务
sudo snap restart multipass
```

#### 虚拟机网络问题

```bash
# 进入虚拟机
multipass shell k8s-master

# 检查网络
ip addr
ping 8.8.8.8
```

### Kubernetes 问题

#### 节点 NotReady

```bash
# 查看节点状态
kubectl get nodes

# 查看节点详情
kubectl describe node k8s-master

# 检查 kubelet 日志
multipass exec k8s-master -- sudo journalctl -u kubelet -f
```

#### Pod 无法启动

```bash
# 查看 Pod 状态
kubectl get pods -A

# 查看 Pod 详情
kubectl describe pod -n poetry-api <pod-name>

# 查看 Pod 日志
kubectl logs -n poetry-api <pod-name>

# 查看事件
kubectl get events -n poetry-api --sort-by='.lastTimestamp'
```

#### 网络问题

```bash
# 检查 Calico Pods
kubectl get pods -n kube-system -l k8s-app=calico-node

# 检查 DNS
kubectl run -it --rm debug --image=busybox --restart=Never -- nslookup kubernetes.default

# 检查 Pod 网络
kubectl run -it --rm debug --image=busybox --restart=Never -- ping 10.244.0.1
```

#### Metrics Server 问题

```bash
# 检查 Metrics Server
kubectl get deployment -n kube-system metrics-server

# 查看日志
kubectl logs -n kube-system deployment/metrics-server

# 测试 metrics
kubectl top nodes
kubectl top pods -A
```

### 常见错误

#### 1. "The connection to the server was refused"

```bash
# 检查 API Server
multipass exec k8s-master -- sudo systemctl status kubelet

# 重启 kubelet
multipass exec k8s-master -- sudo systemctl restart kubelet
```

#### 2. "Unable to connect to the server: dial tcp: lookup"

```bash
# 检查 kubeconfig
cat kubeconfig

# 确保使用正确的 kubeconfig
export KUBECONFIG=$(pwd)/kubeconfig
```

#### 3. "0/3 nodes are available: 3 node(s) had untolerated taint"

```bash
# 检查节点污点
kubectl describe nodes | grep Taints

# 移除 Master 污点（如果需要在 Master 上运行 Pod）
kubectl taint nodes k8s-master node-role.kubernetes.io/control-plane:NoSchedule-
```

## 📚 学习资源

### 实践练习

完成集群搭建后，可以进行以下练习：

1. **Pod 管理**

   ```bash
   # 查看 Pod 分布
   kubectl get pods -n poetry-api -o wide

   # 删除 Pod 观察自动重建
   kubectl delete pod -n poetry-api <pod-name>
   ```

2. **节点管理**

   ```bash
   # 标记节点不可调度
   kubectl cordon k8s-worker1

   # 驱逐节点上的 Pod
   kubectl drain k8s-worker1 --ignore-daemonsets

   # 恢复节点
   kubectl uncordon k8s-worker1
   ```

3. **扩缩容**

   ```bash
   # 手动扩容
   kubectl scale deployment poetry-api -n poetry-api --replicas=5

   # 观察 HPA 自动扩缩容
   kubectl get hpa -n poetry-api -w
   ```

4. **滚动更新**

   ```bash
   # 更新镜像
   kubectl set image deployment/poetry-api -n poetry-api \
     poetry-api=palemoky/chinese-poetry-api:latest

   # 观察更新过程
   kubectl rollout status deployment/poetry-api -n poetry-api
   ```

### 相关文档

- [Kubernetes 官方文档](https://kubernetes.io/docs/)
- [kubeadm 文档](https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/)
- [Calico 文档](https://docs.tigera.io/calico/latest/about/)
- [Multipass 文档](https://multipass.run/docs)

## 🎯 下一步

1. 实践上述练习，掌握 K8s 核心概念
2. 查看 [PRODUCTION.md](PRODUCTION.md) 了解生产环境最佳实践
3. 部署自己的应用到集群

## 💡 提示

- 集群停止后可以随时启动，数据会保留
- 定期备份 kubeconfig 文件
- 使用 `manage-cluster.sh status` 检查集群健康状态
- 虚拟机占用资源较多，不用时可以停止集群

祝您学习愉快！🎉
