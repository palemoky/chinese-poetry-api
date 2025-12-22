#!/bin/bash

#############################################
# Initialize Kubernetes Master Node
# 初始化 Master 节点
#############################################

set -e

POD_NETWORK_CIDR=${1:-"10.244.0.0/16"}

echo "=========================================="
echo "Initializing Kubernetes Master Node"
echo "Pod Network CIDR: $POD_NETWORK_CIDR"
echo "=========================================="

# 获取节点 IP
NODE_IP=$(hostname -I | awk '{print $1}')
echo "Node IP: $NODE_IP"

# 初始化集群
echo ""
echo "[1/4] 初始化 Kubernetes 集群..."
sudo kubeadm init \
    --apiserver-advertise-address="$NODE_IP" \
    --pod-network-cidr="$POD_NETWORK_CIDR" \
    --service-cidr="10.96.0.0/12" \
    --node-name="$(hostname)" \
    --ignore-preflight-errors=NumCPU

# 配置 kubectl
echo ""
echo "[2/4] 配置 kubectl..."
mkdir -p "$HOME/.kube"
sudo cp -f /etc/kubernetes/admin.conf "$HOME/.kube/config"
sudo chown "$(id -u):$(id -g)" "$HOME/.kube/config"

# 验证 API Server 可访问
echo ""
echo "[3/4] 验证 API Server..."
# 节点在网络插件安装前不会 Ready，这里只验证 API Server 可访问
kubectl cluster-info || echo "API Server 启动中，继续..."
sleep 10

# 显示集群信息
echo ""
echo "[4/4] 集群信息:"
kubectl cluster-info
kubectl get nodes

echo ""
echo "=========================================="
echo "Master 节点初始化完成！"
echo "=========================================="
echo ""
echo "Join 命令:"
sudo kubeadm token create --print-join-command
