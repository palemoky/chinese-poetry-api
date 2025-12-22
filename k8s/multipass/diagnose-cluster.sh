#!/bin/bash

#############################################
# Diagnose Cluster Issues
# 诊断集群问题
#############################################

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

echo "=========================================="
echo "  Kubernetes Cluster Diagnostics"
echo "=========================================="
echo ""

# 1. 检查虚拟机状态
log_info "检查虚拟机状态..."
multipass list
echo ""

# 2. 检查 Master 节点
log_info "检查 Master 节点..."
if multipass exec k8s-master -- systemctl is-active kubelet &>/dev/null; then
    log_success "kubelet 运行正常"
else
    log_error "kubelet 未运行"
    log_info "查看 kubelet 日志:"
    multipass exec k8s-master -- sudo journalctl -u kubelet --no-pager -n 50
fi
echo ""

# 3. 检查容器运行时
log_info "检查容器运行时..."
multipass exec k8s-master -- sudo crictl ps -a
echo ""

# 4. 检查 API Server
log_info "检查 API Server..."
if multipass exec k8s-master -- sudo crictl ps | grep kube-apiserver &>/dev/null; then
    log_success "API Server 运行中"

    # 获取 API Server 容器 ID
    APISERVER_ID=$(multipass exec k8s-master -- sudo crictl ps | grep kube-apiserver | awk '{print $1}')

    log_info "API Server 日志（最后 20 行）:"
    multipass exec k8s-master -- sudo crictl logs --tail=20 "$APISERVER_ID"
else
    log_error "API Server 未运行"

    log_info "查找 API Server 容器（包括已停止的）:"
    multipass exec k8s-master -- sudo crictl ps -a | grep kube-apiserver || echo "未找到 API Server 容器"
fi
echo ""

# 5. 检查节点状态
log_info "检查节点状态..."
if [ -f kubeconfig ]; then
    export KUBECONFIG=./kubeconfig
    kubectl get nodes -o wide || log_error "无法获取节点状态"
else
    log_error "kubeconfig 文件不存在"
fi
echo ""

# 6. 检查系统 Pods
log_info "检查系统 Pods..."
if [ -f kubeconfig ]; then
    kubectl get pods -n kube-system || log_error "无法获取 Pod 状态"
else
    log_error "kubeconfig 文件不存在"
fi
echo ""

# 7. 检查资源使用
log_info "检查 Master 节点资源使用..."
multipass exec k8s-master -- free -h
multipass exec k8s-master -- df -h
echo ""

# 8. 检查网络
log_info "检查网络连接..."
multipass exec k8s-master -- ping -c 3 8.8.8.8 || log_warn "外网连接异常"
echo ""

# 9. 提供建议
echo "=========================================="
log_info "诊断建议:"
echo "=========================================="
echo ""

if ! multipass exec k8s-master -- systemctl is-active kubelet &>/dev/null; then
    echo "1. kubelet 未运行，尝试重启:"
    echo "   multipass exec k8s-master -- sudo systemctl restart kubelet"
    echo ""
fi

if [ ! -f kubeconfig ]; then
    echo "2. kubeconfig 不存在，集群可能未初始化成功"
    echo "   建议删除并重新创建集群:"
    echo "   ./manage-cluster.sh delete"
    echo "   ./setup-cluster.sh"
    echo ""
fi

echo "3. 查看详细日志:"
echo "   multipass shell k8s-master"
echo "   sudo journalctl -u kubelet -f"
echo ""

echo "4. 如果问题持续，尝试增加虚拟机资源:"
echo "   编辑 setup-cluster.sh，增加 MASTER_CPU 和 MASTER_MEM"
echo ""
