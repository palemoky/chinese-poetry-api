# 集群安装故障排查

## 问题 1: kubectl: command not found

### 原因

宿主机（Ubuntu 24.04）没有安装 `kubectl`。

### 解决方案

```bash
# 快速安装 kubectl
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl

# 验证安装
kubectl version --client
```

---

## 问题 2: API Server Timeout

### 原因

- 虚拟机启动慢
- 资源不足
- 网络问题

### 解决方案

#### 步骤 1: 删除失败的集群

```bash
cd k8s/multipass
./manage-cluster.sh delete
```

#### 步骤 2: 重新创建集群

```bash
./setup-cluster.sh
```

#### 步骤 3: 如果还是超时，运行诊断

```bash
./diagnose-cluster.sh
```

---

## 诊断工具

### diagnose-cluster.sh

自动检查：

- ✅ 虚拟机状态
- ✅ kubelet 运行状态
- ✅ API Server 日志
- ✅ 资源使用情况
- ✅ 网络连接

---

## 手动诊断步骤

### 1. 检查 kubelet

```bash
multipass shell k8s-master
sudo systemctl status kubelet
sudo journalctl -u kubelet -f
```

### 2. 检查容器

```bash
multipass shell k8s-master
sudo crictl ps -a
sudo crictl logs <container-id>
```

### 3. 检查资源

```bash
multipass exec k8s-master -- free -h
multipass exec k8s-master -- df -h
```

---

## 常见解决方案

### 增加虚拟机资源

编辑 `setup-cluster.sh`:

```bash
MASTER_CPU=6      # 从 4 增加到 6
MASTER_MEM="12G"  # 从 8G 增加到 12G
```

### 重置集群

```bash
./manage-cluster.sh delete
./setup-cluster.sh
```

---

## 完整安装流程

```bash
# 1. 安装 kubectl（宿主机）
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl

# 2. 创建集群
cd k8s/multipass
./setup-cluster.sh

# 3. 如果失败，诊断
./diagnose-cluster.sh

# 4. 如果需要，删除重建
./manage-cluster.sh delete
./setup-cluster.sh
```
