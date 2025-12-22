#!/bin/bash

#############################################
# Install Kubernetes Components
# 在虚拟机内安装 K8s 组件
#############################################

set -e

K8S_VERSION=${1:-"1.28"}

echo "=========================================="
echo "Installing Kubernetes Components"
echo "Version: $K8S_VERSION"
echo "=========================================="

# 禁用 swap
echo "[1/7] 禁用 swap..."
sudo swapoff -a
sudo sed -i '/ swap / s/^/#/' /etc/fstab

# 加载内核模块
echo "[2/7] 加载内核模块..."
cat <<EOF | sudo tee /etc/modules-load.d/k8s.conf
overlay
br_netfilter
EOF

sudo modprobe overlay
sudo modprobe br_netfilter

# 配置内核参数
echo "[3/7] 配置内核参数..."
cat <<EOF | sudo tee /etc/sysctl.d/k8s.conf
net.bridge.bridge-nf-call-iptables  = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward                 = 1
EOF

sudo sysctl --system

# 安装 containerd
echo "[4/7] 安装 containerd 2.0+..."
sudo apt-get update
sudo apt-get install -y containerd

# 配置 containerd
sudo mkdir -p /etc/containerd
containerd config default | sudo tee /etc/containerd/config.toml

# 使用 systemd cgroup driver（K8s 1.35 要求）
sudo sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml

# 确保使用 cgroup v2（K8s 1.35 要求）
if [ ! -f /sys/fs/cgroup/cgroup.controllers ]; then
    echo "警告: 系统未使用 cgroup v2，K8s 1.35 需要 cgroup v2"
fi

# 重启 containerd
sudo systemctl restart containerd
sudo systemctl enable containerd

# 安装 Kubernetes 组件
echo "[5/7] 添加 Kubernetes apt 仓库..."
sudo apt-get update
sudo apt-get install -y apt-transport-https ca-certificates curl gpg

# 添加 Kubernetes GPG 密钥
curl -fsSL https://pkgs.k8s.io/core:/stable:/v${K8S_VERSION}/deb/Release.key | sudo gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg

# 添加 Kubernetes apt 仓库
echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v${K8S_VERSION}/deb/ /" | sudo tee /etc/apt/sources.list.d/kubernetes.list

# 安装 kubelet、kubeadm、kubectl
echo "[6/7] 安装 kubelet、kubeadm、kubectl..."
sudo apt-get update
sudo apt-get install -y kubelet kubeadm kubectl
sudo apt-mark hold kubelet kubeadm kubectl

# 启用 kubelet
sudo systemctl enable kubelet

# 预拉取镜像
echo "[7/7] 预拉取 Kubernetes 镜像..."
sudo kubeadm config images pull

echo ""
echo "=========================================="
echo "Kubernetes 组件安装完成！"
echo "=========================================="
echo ""
echo "已安装版本:"
kubelet --version
kubeadm version
kubectl version --client
