# DynamoDB 表结构升级指南

本文档说明如何将现有的 DynamoDB 表升级到支持 GSI（全局二级索引）的新结构。

## 变更内容

### 新增字段
| 字段名 | 类型 | 说明 |
|--------|------|------|
| `date_partition` | String | 日期分区键，格式: `2025-01-15` |

### 新增 GSI 索引
| 索引名称 | 分区键(PK) | 排序键(SK) | 用途 |
|----------|------------|------------|------|
| `date-index` | `date_partition` | `created_at` | 按日期查询任务 |
| `status-index` | `status` | `created_at` | 按状态查询任务 |

---

## 升级步骤

> **注意**: 以下命令中的 `video-transcode` 和 `ap-southeast-1` 请根据你的实际表名和区域修改。

### 步骤 1: 改成按需计费模式（推荐）

如果你的表使用的是 Provisioned 容量模式，建议先改成按需模式（PAY_PER_REQUEST），这样添加 GSI 时不需要指定容量：

```bash
aws dynamodb update-table \
  --table-name video-transcode \
  --billing-mode PAY_PER_REQUEST \
  --region ap-southeast-1
```

### 步骤 2: 等待表状态变为 ACTIVE

```bash
aws dynamodb describe-table --table-name video-transcode --query "Table.TableStatus" --region ap-southeast-1
```

等返回 `"ACTIVE"` 后，继续下一步。

### 步骤 3: 添加 date-index GSI

```bash
aws dynamodb update-table \
  --table-name video-transcode \
  --attribute-definitions \
    AttributeName=date_partition,AttributeType=S \
    AttributeName=created_at,AttributeType=S \
  --global-secondary-index-updates \
    '[{"Create": {"IndexName": "date-index", "KeySchema": [{"AttributeName": "date_partition", "KeyType": "HASH"}, {"AttributeName": "created_at", "KeyType": "RANGE"}], "Projection": {"ProjectionType": "ALL"}}}]' \
  --region ap-southeast-1
```

### 步骤 4: 等待 date-index 创建完成

```bash
aws dynamodb describe-table --table-name video-transcode --query "Table.GlobalSecondaryIndexes[?IndexName=='date-index'].IndexStatus" --region ap-southeast-1
```

等返回 `["ACTIVE"]` 后（约 5-10 分钟），继续下一步。

### 步骤 5: 添加 status-index GSI

```bash
aws dynamodb update-table \
  --table-name video-transcode \
  --attribute-definitions \
    AttributeName=status,AttributeType=S \
    AttributeName=created_at,AttributeType=S \
  --global-secondary-index-updates \
    '[{"Create": {"IndexName": "status-index", "KeySchema": [{"AttributeName": "status", "KeyType": "HASH"}, {"AttributeName": "created_at", "KeyType": "RANGE"}], "Projection": {"ProjectionType": "ALL"}}}]' \
  --region ap-southeast-1
```

### 步骤 6: 等待 status-index 创建完成

```bash
aws dynamodb describe-table --table-name video-transcode --query "Table.GlobalSecondaryIndexes[?IndexName=='status-index'].IndexStatus" --region ap-southeast-1
```

等返回 `["ACTIVE"]` 后，GSI 创建完成。

---

## 迁移现有数据

现有数据缺少 `date_partition` 字段，需要补充。运行以下脚本：

```bash
# 创建迁移脚本
cat > migrate_dynamodb.sh << 'EOF'
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
EOF

chmod +x migrate_dynamodb.sh
./migrate_dynamodb.sh
```

---

## 验证迁移结果

```bash
# 检查 GSI 状态
aws dynamodb describe-table \
  --table-name video-transcode \
  --query "Table.GlobalSecondaryIndexes" \
  --region ap-southeast-1

# 测试 date-index 查询
aws dynamodb query \
  --table-name video-transcode \
  --index-name date-index \
  --key-condition-expression "date_partition = :date" \
  --expression-attribute-values '{":date": {"S": "2025-12-10"}}' \
  --region ap-southeast-1

# 测试 status-index 查询
aws dynamodb query \
  --table-name video-transcode \
  --index-name status-index \
  --key-condition-expression "#s = :status" \
  --expression-attribute-names '{"#s": "status"}' \
  --expression-attribute-values '{":status": {"S": "completed"}}' \
  --region ap-southeast-1
```

---

## 回滚方案

如果需要回滚，可以删除 GSI（不影响主表数据）：

```bash
# 删除 date-index
aws dynamodb update-table \
  --table-name video-transcode \
  --global-secondary-index-updates '[{"Delete": {"IndexName": "date-index"}}]' \
  --region ap-southeast-1

# 删除 status-index
aws dynamodb update-table \
  --table-name video-transcode \
  --global-secondary-index-updates '[{"Delete": {"IndexName": "status-index"}}]' \
  --region ap-southeast-1
```

注意：`date_partition` 字段会保留在数据中，但不会影响旧版本代码运行。

---

## 备选方案：Provisioned 模式下添加 GSI

如果你不想改成按需模式，可以在添加 GSI 时指定读写容量：

```bash
aws dynamodb update-table \
  --table-name video-transcode \
  --attribute-definitions \
    AttributeName=date_partition,AttributeType=S \
    AttributeName=created_at,AttributeType=S \
  --global-secondary-index-updates \
    '[{
      "Create": {
        "IndexName": "date-index",
        "KeySchema": [
          {"AttributeName": "date_partition", "KeyType": "HASH"},
          {"AttributeName": "created_at", "KeyType": "RANGE"}
        ],
        "Projection": {"ProjectionType": "ALL"},
        "ProvisionedThroughput": {
          "ReadCapacityUnits": 5,
          "WriteCapacityUnits": 5
        }
      }
    }]' \
  --region ap-southeast-1
```

---

## 注意事项

1. **GSI 创建时间**: 每个 GSI 创建需要 5-10 分钟，期间表仍可正常读写
2. **一次只能创建一个 GSI**: DynamoDB 限制同时只能创建一个 GSI，必须等前一个完成
3. **成本**: GSI 会增加存储和写入成本（相当于维护一份数据副本）
4. **兼容性**: 新代码兼容旧数据（没有 `date_partition` 的记录会在 Scan 时正常返回）
5. **按需模式**: 推荐使用 PAY_PER_REQUEST 模式，省心且适合波动负载
