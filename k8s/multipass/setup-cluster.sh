#!/bin/bash

#############################################
# Kubernetes Cluster Setup with Multipass
# 一键创建生产级 K8s 集群
#############################################

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 配置参数
MASTER_NAME="k8s-master"
WORKER1_NAME="k8s-worker1"
WORKER2_NAME="k8s-worker2"

MASTER_CPU=4
MASTER_MEM="8G"
MASTER_DISK="40G"

WORKER_CPU=4
WORKER_MEM="8G"
WORKER_DISK="40G"

K8S_VERSION="1.34"
POD_NETWORK_CIDR="10.244.0.0/16"

# 日志函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查依赖
check_dependencies() {
    log_info "检查依赖..."

    if ! command -v multipass &> /dev/null; then
        log_error "Multipass 未安装"
        log_info "请运行: sudo snap install multipass"
        exit 1
    fi

    log_success "依赖检查通过"
}

# 创建虚拟机
create_vm() {
    local name=$1
    local cpu=$2
    local mem=$3
    local disk=$4

    log_info "创建虚拟机: $name (CPU: $cpu, MEM: $mem, DISK: $disk)"

    if multipass list | grep -q "$name"; then
        log_warn "虚拟机 $name 已存在，跳过创建"
        return
    fi

    multipass launch --name "$name" \
        --cpus "$cpu" \
        --memory "$mem" \
        --disk "$disk" \
        --cloud-init - <<EOF
#cloud-config
package_update: true
package_upgrade: true
packages:
  - apt-transport-https
  - ca-certificates
  - curl
  - gnupg
  - lsb-release
runcmd:
  - echo "VM $name initialized" > /tmp/cloud-init-done
EOF

    log_success "虚拟机 $name 创建成功"
}

# 等待虚拟机就绪
wait_for_vm() {
    local name=$1
    log_info "等待虚拟机 $name 就绪..."

    local max_attempts=30
    local attempt=0

    while [ $attempt -lt $max_attempts ]; do
        if multipass exec "$name" -- test -f /tmp/cloud-init-done 2>/dev/null; then
            log_success "虚拟机 $name 已就绪"
            return 0
        fi

        attempt=$((attempt + 1))
        echo -n "."
        sleep 2
    done

    log_error "虚拟机 $name 启动超时"
    return 1
}

# 安装 Kubernetes 组件
install_k8s_components() {
    local name=$1
    log_info "在 $name 上安装 Kubernetes 组件..."

    multipass transfer install-k8s.sh "$name:/tmp/"
    multipass exec "$name" -- sudo bash /tmp/install-k8s.sh "$K8S_VERSION"

    log_success "$name 上的 Kubernetes 组件安装完成"
}

# 初始化 Master 节点
init_master() {
    log_info "初始化 Master 节点..."

    multipass transfer init-master.sh "$MASTER_NAME:/tmp/"
    multipass exec "$MASTER_NAME" -- sudo bash /tmp/init-master.sh "$POD_NETWORK_CIDR"

    # 获取 kubeconfig
    log_info "获取 kubeconfig..."
    multipass exec "$MASTER_NAME" -- sudo cat /etc/kubernetes/admin.conf > kubeconfig

    # 获取 join 命令
    log_info "获取 join 命令..."
    multipass exec "$MASTER_NAME" -- sudo kubeadm token create --print-join-command > join-command.sh
    chmod +x join-command.sh

    log_success "Master 节点初始化完成"
}

# 加入 Worker 节点
join_worker() {
    local name=$1
    log_info "将 $name 加入集群..."

    multipass transfer join-command.sh "$name:/tmp/"
    multipass exec "$name" -- sudo bash /tmp/join-command.sh

    log_success "$name 已加入集群"
}

# 安装网络插件
install_network_plugin() {
    log_info "安装 Calico 网络插件..."

    export KUBECONFIG=./kubeconfig
    kubectl apply -f https://raw.githubusercontent.com/projectcalico/calico/v3.31.1/manifests/calico.yaml

    log_success "Calico 网络插件安装完成"
}

# 安装 Ingress Controller
install_ingress() {
    log_info "安装 Nginx Ingress Controller..."

    export KUBECONFIG=./kubeconfig
    kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.14.0/deploy/static/provider/baremetal/deploy.yaml

    log_success "Nginx Ingress Controller 安装完成"
}

# 安装 MetalLB
install_metallb() {
    log_info "安装 MetalLB..."

    export KUBECONFIG=./kubeconfig

    # 安装 MetalLB
    kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.15.2/config/manifests/metallb-native.yaml

    # 等待 MetalLB 就绪
    log_info "等待 MetalLB 就绪..."
    kubectl wait --namespace metallb-system \
        --for=condition=ready pod \
        --selector=app=metallb \
        --timeout=180s || true

    # 等待 webhook 服务就绪（关键！）
    log_info "等待 MetalLB webhook 就绪..."
    kubectl wait --namespace metallb-system \
        --for=condition=available deployment/controller \
        --timeout=120s || true

    # 额外等待 webhook 完全启动
    sleep 30

    # 获取虚拟机 IP 范围
    local master_ip=$(multipass info "$MASTER_NAME" | grep IPv4 | awk '{print $2}')
    local ip_base=$(echo "$master_ip" | cut -d. -f1-3)
    local ip_start="${ip_base}.200"
    local ip_end="${ip_base}.250"

    log_info "配置 MetalLB IP 池: $ip_start - $ip_end"

    # 创建 IP 地址池
    cat <<EOF | kubectl apply -f -
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: default-pool
  namespace: metallb-system
spec:
  addresses:
  - ${ip_start}-${ip_end}
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: default
  namespace: metallb-system
spec:
  ipAddressPools:
  - default-pool
EOF

    log_success "MetalLB 安装完成"
}

# 安装 Metrics Server
install_metrics_server() {
    log_info "安装 Metrics Server..."

    export KUBECONFIG=./kubeconfig
    kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml

    # 修补 Metrics Server（允许不安全的 TLS）
    kubectl patch deployment metrics-server -n kube-system --type='json' \
        -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--kubelet-insecure-tls"}]' || true

    log_success "Metrics Server 安装完成"
}

# 验证集群
verify_cluster() {
    log_info "验证集群状态..."

    export KUBECONFIG=./kubeconfig

    echo ""
    log_info "节点状态:"
    kubectl get nodes -o wide

    echo ""
    log_info "系统 Pods:"
    kubectl get pods -A

    echo ""
    log_info "等待所有节点就绪..."
    kubectl wait --for=condition=Ready nodes --all --timeout=300s

    log_success "集群验证完成"
}

# 显示集群信息
show_cluster_info() {
    echo ""
    echo "=========================================="
    log_success "Kubernetes 集群创建成功！"
    echo "=========================================="
    echo ""

    log_info "集群信息:"
    echo "  Master: $MASTER_NAME"
    echo "  Worker: $WORKER1_NAME, $WORKER2_NAME"
    echo ""

    log_info "访问集群:"
    echo "  export KUBECONFIG=$(pwd)/kubeconfig"
    echo "  kubectl get nodes"
    echo ""

    log_info "虚拟机管理:"
    echo "  查看虚拟机: multipass list"
    echo "  进入虚拟机: multipass shell $MASTER_NAME"
    echo "  停止集群: ./manage-cluster.sh stop"
    echo "  启动集群: ./manage-cluster.sh start"
    echo "  删除集群: ./manage-cluster.sh delete"
    echo ""

    log_info "部署应用:"
    echo "  ./deploy-app.sh"
    echo ""
}

# 主函数
main() {
    echo "=========================================="
    echo "  Kubernetes Cluster Setup"
    echo "  Multipass + kubeadm"
    echo "=========================================="
    echo ""

    # 检查依赖
    check_dependencies

    # 创建虚拟机
    log_info "步骤 1/8: 创建虚拟机"
    create_vm "$MASTER_NAME" "$MASTER_CPU" "$MASTER_MEM" "$MASTER_DISK"
    create_vm "$WORKER1_NAME" "$WORKER_CPU" "$WORKER_MEM" "$WORKER_DISK"
    create_vm "$WORKER2_NAME" "$WORKER_CPU" "$WORKER_MEM" "$WORKER_DISK"

    # 等待虚拟机就绪
    log_info "步骤 2/8: 等待虚拟机就绪"
    wait_for_vm "$MASTER_NAME"
    wait_for_vm "$WORKER1_NAME"
    wait_for_vm "$WORKER2_NAME"

    # 安装 K8s 组件
    log_info "步骤 3/8: 安装 Kubernetes 组件"
    install_k8s_components "$MASTER_NAME"
    install_k8s_components "$WORKER1_NAME"
    install_k8s_components "$WORKER2_NAME"

    # 初始化 Master
    log_info "步骤 4/8: 初始化 Master 节点"
    init_master

    # 加入 Worker 节点
    log_info "步骤 5/8: 加入 Worker 节点"
    join_worker "$WORKER1_NAME"
    join_worker "$WORKER2_NAME"

    # 安装网络插件
    log_info "步骤 6/8: 安装网络插件"
    install_network_plugin

    # 安装附加组件
    log_info "步骤 7/8: 安装附加组件"
    install_metrics_server
    install_ingress
    install_metallb

    # 验证集群
    log_info "步骤 8/8: 验证集群"
    verify_cluster

    # 显示集群信息
    show_cluster_info
}

# 运行主函数
main "$@"
