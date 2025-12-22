#!/bin/bash

#############################################
# Cluster Management Script
# 集群管理脚本
#############################################

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

MASTER_NAME="k8s-master"
WORKER1_NAME="k8s-worker1"
WORKER2_NAME="k8s-worker2"

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 显示帮助
show_help() {
    cat <<EOF
Kubernetes 集群管理工具

用法: $0 <command>

命令:
  start       启动集群所有虚拟机
  stop        停止集群所有虚拟机
  restart     重启集群所有虚拟机
  delete      删除集群（包括所有虚拟机）
  status      查看集群状态
  info        显示集群信息
  shell       进入 Master 节点
  logs        查看节点日志
  help        显示此帮助信息

示例:
  $0 start        # 启动集群
  $0 status       # 查看状态
  $0 shell        # 进入 Master 节点
EOF
}

# 启动集群
start_cluster() {
    log_info "启动集群..."

    multipass start "$MASTER_NAME" || true
    multipass start "$WORKER1_NAME" || true
    multipass start "$WORKER2_NAME" || true

    log_success "集群已启动"

    # 等待节点就绪
    if [ -f kubeconfig ]; then
        log_info "等待节点就绪..."
        sleep 10
        export KUBECONFIG=./kubeconfig
        kubectl get nodes
    fi
}

# 停止集群
stop_cluster() {
    log_info "停止集群..."

    multipass stop "$MASTER_NAME" || true
    multipass stop "$WORKER1_NAME" || true
    multipass stop "$WORKER2_NAME" || true

    log_success "集群已停止"
}

# 重启集群
restart_cluster() {
    log_info "重启集群..."
    stop_cluster
    sleep 5
    start_cluster
}

# 删除集群
delete_cluster() {
    echo -e "${YELLOW}警告: 此操作将删除所有虚拟机和数据！${NC}"
    read -p "确认删除集群? (yes/no): " confirm

    if [ "$confirm" != "yes" ]; then
        log_info "取消删除"
        return
    fi

    log_info "删除集群..."

    multipass delete "$MASTER_NAME" || true
    multipass delete "$WORKER1_NAME" || true
    multipass delete "$WORKER2_NAME" || true

    multipass purge

    # 清理本地文件
    rm -f kubeconfig join-command.sh

    log_success "集群已删除"
}

# 查看集群状态
show_status() {
    log_info "虚拟机状态:"
    multipass list

    echo ""
    if [ -f kubeconfig ]; then
        log_info "Kubernetes 节点状态:"
        export KUBECONFIG=./kubeconfig
        kubectl get nodes -o wide || log_error "无法连接到集群"

        echo ""
        log_info "系统 Pods:"
        kubectl get pods -A || true
    else
        log_error "kubeconfig 文件不存在，集群可能未初始化"
    fi
}

# 显示集群信息
show_info() {
    log_info "集群配置:"
    echo "  Master: $MASTER_NAME"
    echo "  Worker: $WORKER1_NAME, $WORKER2_NAME"
    echo ""

    if [ -f kubeconfig ]; then
        export KUBECONFIG=./kubeconfig

        log_info "集群信息:"
        kubectl cluster-info || true

        echo ""
        log_info "虚拟机 IP 地址:"
        for vm in "$MASTER_NAME" "$WORKER1_NAME" "$WORKER2_NAME"; do
            ip=$(multipass info "$vm" | grep IPv4 | awk '{print $2}')
            echo "  $vm: $ip"
        done
    fi
}

# 进入 Master 节点
shell_master() {
    log_info "进入 Master 节点..."
    multipass shell "$MASTER_NAME"
}

# 查看日志
show_logs() {
    local node=${1:-$MASTER_NAME}

    log_info "查看 $node 日志..."
    multipass exec "$node" -- sudo journalctl -u kubelet -f
}

# 主函数
main() {
    local command=${1:-help}

    case "$command" in
        start)
            start_cluster
            ;;
        stop)
            stop_cluster
            ;;
        restart)
            restart_cluster
            ;;
        delete)
            delete_cluster
            ;;
        status)
            show_status
            ;;
        info)
            show_info
            ;;
        shell)
            shell_master
            ;;
        logs)
            show_logs "$2"
            ;;
        help|--help|-h)
            show_help
            ;;
        *)
            log_error "未知命令: $command"
            echo ""
            show_help
            exit 1
            ;;
    esac
}

main "$@"
