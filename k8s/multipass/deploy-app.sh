#!/bin/bash

#############################################
# Deploy Poetry API to Kubernetes
# 部署诗词 API 到 K8s 集群
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

# 检查 kubeconfig
check_kubeconfig() {
    if [ ! -f kubeconfig ]; then
        log_error "kubeconfig 文件不存在"
        log_info "请先运行 ./setup-cluster.sh 创建集群"
        exit 1
    fi

    export KUBECONFIG=./kubeconfig
}

# 创建数据目录
create_data_dir() {
    log_info "在所有节点创建数据目录..."

    for node in k8s-master k8s-worker1 k8s-worker2; do
        multipass exec "$node" -- sudo mkdir -p /mnt/data/poetry-api
        multipass exec "$node" -- sudo chmod 777 /mnt/data/poetry-api
    done

    log_success "数据目录创建完成"
}

# 部署应用
deploy_app() {
    log_info "部署 Poetry API..."

    # 应用 K8s 配置（按顺序）
    local k8s_dir="../"

    # 1. 创建 Namespace
    kubectl apply -f "${k8s_dir}namespace.yaml"

    # 2. 创建 ConfigMap 和 Secret
    kubectl apply -f "${k8s_dir}configmap.yaml"
    kubectl apply -f "${k8s_dir}secret.yaml"

    # 3. 创建 PV 和 PVC
    kubectl apply -f "${k8s_dir}persistent-volume.yaml"

    # 4. 部署应用
    kubectl apply -f "${k8s_dir}deployment.yaml"
    kubectl apply -f "${k8s_dir}service.yaml"

    # 5. 创建 HPA
    kubectl apply -f "${k8s_dir}hpa.yaml"

    # 6. 创建 Ingress（可选）
    kubectl apply -f "${k8s_dir}ingress.yaml" || true

    log_success "应用部署完成"
}

# 等待 Pod 就绪
wait_for_pods() {
    log_info "等待 Pods 就绪..."

    kubectl wait --for=condition=ready pod \
        -l app=chinese-poetry-api \
        -n poetry-api \
        --timeout=300s || true

    log_success "Pods 已就绪"
}

# 显示部署信息
show_deployment_info() {
    echo ""
    echo "=========================================="
    log_success "Poetry API 部署成功！"
    echo "=========================================="
    echo ""

    log_info "Pods 状态:"
    kubectl get pods -n poetry-api -o wide

    echo ""
    log_info "Services:"
    kubectl get svc -n poetry-api

    echo ""
    log_info "访问应用:"

    # 获取 NodePort
    local nodeport=$(kubectl get svc -n poetry-api poetry-api-nodeport -o jsonpath='{.spec.ports[0].nodePort}')
    local master_ip=$(multipass info k8s-master | grep IPv4 | awk '{print $2}')

    echo "  NodePort: http://${master_ip}:${nodeport}"
    echo "  测试: curl http://${master_ip}:${nodeport}/api/v1/poems/random"

    # 检查 LoadBalancer
    local lb_ip=$(kubectl get svc -n poetry-api poetry-api -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || echo "")
    if [ -n "$lb_ip" ]; then
        echo "  LoadBalancer: http://${lb_ip}"
        echo "  测试: curl http://${lb_ip}/api/v1/poems/random"
    fi

    echo ""
    log_info "查看日志:"
    echo "  kubectl logs -n poetry-api -l app=chinese-poetry-api --tail=50"

    echo ""
    log_info "进入 Pod:"
    echo "  kubectl exec -it -n poetry-api deployment/poetry-api -- sh"

    echo ""
}

# 主函数
main() {
    echo "=========================================="
    echo "  Deploy Poetry API"
    echo "=========================================="
    echo ""

    # 检查 kubeconfig
    check_kubeconfig

    # 创建数据目录
    create_data_dir

    # 部署应用
    deploy_app

    # 等待 Pods 就绪
    wait_for_pods

    # 显示部署信息
    show_deployment_info
}

main "$@"
