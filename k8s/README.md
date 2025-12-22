# Kubernetes 部署指南

使用 **Multipass + kubeadm** 搭建生产级 K8s 集群（1 Master + 2 Worker）。

## 🚀 快速开始

```bash
# 1. 安装 Multipass
sudo snap install multipass

# 2. 创建集群（10-15 分钟）
cd k8s/multipass
./setup-cluster.sh

# 3. 部署应用
./deploy-app.sh
```

## 📚 文档导航

- **[multipass/README.md](multipass/README.md)** - 完整部署指南和学习路径 ⭐ 主要文档
- **[PRODUCTION.md](multipass/PRODUCTION.md)** - 生产环境最佳实践

## 🏗️ 集群架构

| 节点        | CPU | 内存 | 组件                        |
| ----------- | --- | ---- | --------------------------- |
| k8s-master  | 4   | 8GB  | API Server, etcd, Scheduler |
| k8s-worker1 | 4   | 8GB  | kubelet, Calico             |
| k8s-worker2 | 4   | 8GB  | kubelet, Calico             |

**网络和插件**：

- Calico 网络（Pod 网络）
- Nginx Ingress（HTTP 路由）
- MetalLB（LoadBalancer）
- Metrics Server（监控）

## 🎯 学习路径

### 第 1 周：基础操作

- Pod、Deployment、Service
- 跨节点调度和负载均衡
- 数据持久化

### 第 2 周：节点管理

- cordon、drain、uncordon
- Pod 迁移和故障恢复
- 节点资源管理

### 第 3 周：自动化

- HPA 自动扩缩容
- 滚动更新和回滚
- Job 和 CronJob

### 第 4 周：网络和安全

- Service 类型（ClusterIP、NodePort、LoadBalancer）
- Ingress 配置
- NetworkPolicy

详见 [multipass/README.md](multipass/README.md)

## 🔧 集群管理

```bash
cd k8s/multipass

# 查看状态
./manage-cluster.sh status

# 停止集群
./manage-cluster.sh stop

# 启动集群
./manage-cluster.sh start

# 删除集群
./manage-cluster.sh delete
```

## 💡 常见问题

**Q: 会影响宿主机吗？**
A: 不会。所有修改都在虚拟机内部。

**Q: 资源占用？**
A: 24GB 内存 + 120GB 磁盘。不用时可停止集群。

**Q: 如何访问应用？**
A: NodePort (30127) 或 LoadBalancer IP。

## 📂 配置文件说明

```
k8s/
├── README.md                    # 本文件
├── multipass/                   # 🌟 主要学习环境
│   ├── README.md                # 完整指南（核心文档）
│   ├── PRODUCTION.md            # 生产最佳实践
│   ├── setup-cluster.sh         # 一键创建集群
│   ├── manage-cluster.sh        # 集群管理
│   └── deploy-app.sh            # 应用部署
│
└── [YAML 配置文件]              # K8s 资源定义
    ├── namespace.yaml
    ├── deployment.yaml
    ├── service.yaml
    ├── hpa.yaml
    └── ...
```

## 🎓 下一步

1. 运行 `./setup-cluster.sh` 创建集群
2. 阅读 [multipass/README.md](multipass/README.md) 学习核心概念
3. 实践 Pod 管理、节点调度、故障恢复
4. 查看 [PRODUCTION.md](multipass/PRODUCTION.md) 了解生产环境配置
