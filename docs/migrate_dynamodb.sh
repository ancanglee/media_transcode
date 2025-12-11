#!/bin/bash

TABLE_NAME="video-transcode"
REGION="ap-southeast-1"

echo "开始迁移 DynamoDB 数据..."

# 扫描所有没有 date_partition 的记录
aws dynamodb scan \
  --table-name $TABLE_NAME \
  --filter-expression "attribute_not_exists(date_partition)" \
  --projection-expression "task_id, created_at" \
  --region $REGION \
  --output json | jq -r '.Items[] | @base64' | while read item; do
  
  # 解码 item
  decoded=$(echo $item | base64 -d)
  task_id=$(echo $decoded | jq -r '.task_id.S')
  created_at=$(echo $decoded | jq -r '.created_at.S')
  
  # 从 created_at 提取日期部分 (格式: 2025-01-15T10:30:00Z -> 2025-01-15)
  date_partition=$(echo $created_at | cut -d'T' -f1)
  
  if [ -z "$date_partition" ] || [ "$date_partition" == "null" ]; then
    # 如果 created_at 格式不对，使用当前日期
    date_partition=$(date +%Y-%m-%d)
  fi
  
  echo "更新任务: $task_id -> date_partition: $date_partition"
  
  # 更新记录
  aws dynamodb update-item \
    --table-name $TABLE_NAME \
    --key "{\"task_id\": {\"S\": \"$task_id\"}}" \
    --update-expression "SET date_partition = :dp" \
    --expression-attribute-values "{\":dp\": {\"S\": \"$date_partition\"}}" \
    --region $REGION
done

echo "迁移完成!"
