#!/bin/bash

# 创建用户表的脚本
# 使用方法: ./create_user_table.sh [表名] [区域]

TABLE_NAME=${1:-"video-transcode-users"}
REGION=${2:-"us-west-2"}

echo "🔧 创建 DynamoDB 用户表: $TABLE_NAME"
echo "📍 区域: $REGION"

# 创建表
aws dynamodb create-table \
    --table-name "$TABLE_NAME" \
    --attribute-definitions \
        AttributeName=username,AttributeType=S \
    --key-schema \
        AttributeName=username,KeyType=HASH \
    --billing-mode PAY_PER_REQUEST \
    --region "$REGION"

if [ $? -eq 0 ]; then
    echo "✅ 用户表创建成功"
    echo ""
    echo "📋 表结构:"
    echo "  - 主键: username (String)"
    echo "  - 字段: password, role, created_at, updated_at"
    echo ""
    echo "🔐 默认管理员账户将在服务启动时自动创建:"
    echo "  - 用户名: admin"
    echo "  - 密码: admin"
    echo ""
    echo "⚠️  请在生产环境中修改默认密码!"
else
    echo "❌ 创建表失败，请检查 AWS 凭证和权限"
fi
