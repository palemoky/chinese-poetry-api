#!/bin/bash

#############################################
# Deploy NFS PersistentVolume
# 自动获取 Master IP 并部署 NFS PV
#############################################

set -e

# 颜色定义
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

# 获取 Master 节点 IP
log_info "获取 Master 节点 IP..."
MASTER_IP=$(multipass info k8s-master | grep IPv4 | awk '{print $2}')

if [ -z "$MASTER_IP" ]; then
    echo "错误: 无法获取 Master 节点 IP"
    exit 1
fi

log_info "Master 节点 IP: $MASTER_IP"

# 创建临时文件
TEMP_FILE=$(mktemp)

# 替换 IP 地址
log_info "生成 NFS PV 配置..."
sed "s/192.168.64.2/$MASTER_IP/g" persistent-volume-nfs.yaml > "$TEMP_FILE"

# 应用配置
log_info "部署 NFS PersistentVolume..."
kubectl apply -f "$TEMP_FILE"

# 清理临时文件
rm -f "$TEMP_FILE"

log_success "NFS PV 部署完成！"

# 显示结果
echo ""
kubectl get pv poetry-data-pv-nfs
