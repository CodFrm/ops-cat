# MySQL Table Editor Manual Test Checklist

用于手动验收 `2026-04-25-mysql-table-editor.md` 中自动化测试不能完全覆盖的真实桌面链路：Wails IPC、真实 MySQL 连接、剪贴板、文件下载/上传、SQL 确认弹窗和加载停止体验。

## 测试原则

- 只连接本地或测试 MySQL，不要连接生产库。
- 如需启动 MySQL，优先复用本机已有共享 MySQL 服务或共享 Docker Compose 栈，不要为 OpsKat 单独新增项目级数据库容器。
- 删除、导入、提交修改前必须看到 SQL 预览或确认弹窗；任何静默写库都算失败。
- 每个失败项请记录：操作步骤、实际结果、截图或录屏、控制台错误。

## 环境准备

1. 启动应用：

```bash
make dev
```

2. 准备一个 MySQL 测试资产，连接到测试库，例如 `opskat_manual_test`。

3. 在测试库执行以下 SQL，准备有主键表和无主键表：

```sql
DROP DATABASE IF EXISTS opskat_manual_test;
CREATE DATABASE opskat_manual_test CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE opskat_manual_test;

CREATE TABLE users (
  id INT PRIMARY KEY AUTO_INCREMENT,
  name VARCHAR(64) NOT NULL,
  email VARCHAR(128) NULL,
  note TEXT NULL,
  amount DECIMAL(10,2) NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO users (name, email, note, amount) VALUES
  ('Alice', 'alice@example.com', 'hello, world', 12.30),
  ('Bob', NULL, '', 0),
  ('中文用户', 'zh@example.com', '包含中文、逗号, 和引号 ''quote''', 88.88);

CREATE TABLE no_pk_users (
  name VARCHAR(64),
  email VARCHAR(128),
  note TEXT NULL,
  amount DECIMAL(10,2) NULL
);

INSERT INTO no_pk_users (name, email, note, amount) VALUES
  ('NoPK Alice', 'nopk-a@example.com', NULL, 1.00),
  ('NoPK Bob', 'nopk-b@example.com', 'same row fallback', 2.00);
```

4. 准备导入文件。

`users-import.csv`：

```csv
name,email,note,amount
Imported Alice,import-a@example.com,"line
break",101.01
Imported 中文,import-zh@example.com,"中文, comma, ""quote""",202.02
Imported Null,NULL,,303.03
```

`users-import-mismatch.csv`：

```csv
external_id,full_name
1,Mismatch Alice
```

## 手动测试清单

### 1. 打开表数据页

- [ ] 在资产树中打开 MySQL 测试资产。
- [ ] 展开 `opskat_manual_test`。
- [ ] 打开 `users` 表数据页。

期望：
- [ ] 表格显示 `users` 数据。
- [ ] 顶部有 WHERE / ORDER BY 输入框。
- [ ] 顶部工具栏有新增、删除、提交、放弃、刷新、停止、导入、导出、SQL 预览、显示设置入口。
- [ ] 底部状态栏显示 pending edits、分页、刷新/停止按钮。

### 2. 右键基础编辑动作

在 `users` 表中右键任意可编辑单元格。

- [ ] 菜单显示 Copy Value。
- [ ] 菜单显示 Copy Field Name。
- [ ] 菜单显示 Paste。
- [ ] 菜单显示 Set to Empty String。
- [ ] 菜单显示 Set to NULL。
- [ ] 菜单显示 Refresh。

继续验证：
- [ ] Copy Value 后系统剪贴板内容等于当前单元格值。
- [ ] Copy Field Name 后系统剪贴板内容等于当前列名。
- [ ] Paste 会把剪贴板文本写入当前单元格 pending edit，不直接提交数据库。
- [ ] Set to Empty String 后底部 pending edits 数量增加，单元格显示编辑态。
- [ ] Set to NULL 后底部 pending edits 数量增加，SQL 预览中该字段值为 `NULL`。

### 3. 编辑提交、预览和放弃

- [ ] 双击 `Alice` 的 `note` 单元格，改成 `manual quote ' 中文`。
- [ ] 点击 SQL 预览。
- [ ] 确认 SQL 使用 `UPDATE`，并且 `WHERE` 优先只使用主键 `id`。
- [ ] 关闭预览后点击放弃修改。
- [ ] pending edits 归零，单元格恢复原值。
- [ ] 再次编辑同一单元格并点击提交修改。
- [ ] 提交前出现 SQL 确认弹窗。
- [ ] 确认执行后 toast 显示影响行数。
- [ ] 刷新后新值仍存在。

### 4. 右键筛选、排序和清除

- [ ] 右键 `email` 为 `NULL` 的单元格，选择按此值筛选。
- [ ] WHERE 输入框变成类似 `` `email` IS NULL ``。
- [ ] 表格刷新后只显示 NULL email 记录。
- [ ] 右键 `amount` 列任意单元格，选择升序排序。
- [ ] ORDER 状态生效，第一页刷新，表头排序方向可见。
- [ ] 右键单元格选择清除筛选和排序。
- [ ] WHERE / ORDER BY / 表头排序状态清空，数据恢复未筛选状态。

### 5. 删除记录确认

有主键表：
- [ ] 选中 `users` 中一行，点击工具栏删除。
- [ ] 删除前出现 SQL 确认弹窗。
- [ ] SQL 形如 `DELETE FROM `opskat_manual_test`.`users` WHERE `id` = '...' LIMIT 1;`。
- [ ] 确认后 toast 显示删除影响行数。
- [ ] 当前页和总数刷新，该行消失。

无主键表：
- [ ] 打开 `no_pk_users` 表。
- [ ] 右键一行选择 Delete Record。
- [ ] 删除确认弹窗显示风险提示：无主键，将匹配可见列并 LIMIT 1。
- [ ] SQL WHERE 包含所有可见列值，且包含 `LIMIT 1`。
- [ ] 取消删除时数据库不变。

### 6. 顶部工具栏和底部状态栏

- [ ] 无选中行时删除按钮禁用。
- [ ] 有选中行时删除按钮启用。
- [ ] 无 pending edits 时提交、放弃、SQL 预览禁用。
- [ ] 有 pending edits 时提交、放弃、SQL 预览启用。
- [ ] 刷新按钮可刷新当前页。
- [ ] 分页按钮、页码输入、每页数量切换保持原行为。
- [ ] pending edits 摘要显示第一条待执行 SQL。

### 7. 导出 CSV / TSV / INSERT SQL

在 `users` 表中测试当前页导出。

- [ ] 选择 CSV 并导出，浏览器/系统下载文件。
- [ ] CSV 包含表头。
- [ ] CSV 正确处理中文、逗号、换行、双引号。
- [ ] NULL 在 CSV 中为空单元格。
- [ ] 选择 TSV 并导出，内容使用 tab 分隔。
- [ ] 选择 SQL 并导出，文件内容为 `INSERT INTO ...`。
- [ ] SQL 中字符串单引号被转义，NULL 使用 SQL `NULL`。

### 8. 导入 CSV/TSV 预览和确认

正常导入：
- [ ] 点击导入，选择 `users-import.csv`。
- [ ] 弹窗识别 CSV。
- [ ] 预览区显示前 20 行内的数据。
- [ ] 同名字段自动映射到当前表字段。
- [ ] 可切换 NULL 策略。
- [ ] 点击预览变更后出现 SQL 确认弹窗。
- [ ] SQL 为 INSERT，不是 UPDATE/UPSERT。
- [ ] 确认执行后 toast 显示导入影响行数。
- [ ] 表格刷新后能看到导入的新行。

字段不匹配：
- [ ] 再次点击导入，选择 `users-import-mismatch.csv`。
- [ ] 弹窗明确提示上传字段与当前表不匹配，需要至少映射一个字段。
- [ ] SQL 预览按钮保持禁用。
- [ ] 手动把一个上传字段映射到表字段后，不匹配提示变成“未映射字段将跳过”类提示。
- [ ] 有至少一个映射后可以进入 SQL 预览。

### 9. Stop Loading

这个场景需要一个较慢查询。如果普通刷新太快，可临时在 WHERE 中输入能导致慢查询的表达式，例如：

```sql
SLEEP(3) = 0
```

检查：
- [ ] 点击刷新后 loading 状态出现，停止按钮启用。
- [ ] 点击停止，toast 提示已停止等待当前操作结果。
- [ ] 旧请求返回后不会覆盖当前表格数据。
- [ ] 再次点击刷新，新请求结果可以正常显示。

### 10. Copy As 和 UUID

- [ ] 右键任意单元格选择 Generate UUID。
- [ ] 当前单元格出现 UUID pending edit。
- [ ] SQL 预览可看到该 UUID 值。
- [ ] 右键选择 Copy As -> INSERT Statement。
- [ ] 剪贴板内容为当前行 INSERT SQL。
- [ ] 右键选择 Copy As -> UPDATE Statement。
- [ ] 有主键时 UPDATE WHERE 使用主键。
- [ ] 右键选择 TSV Data / TSV Fields / TSV Fields and Data。
- [ ] 剪贴板内容分别符合仅数据、仅字段名、字段名加数据。

### 11. 表格显示设置

- [ ] 点击显示设置。
- [ ] 隐藏 `note` 列后，表格不显示该列。
- [ ] 隐藏列不影响编辑、删除、导入导出 SQL 构造中的原始 rows 数据。
- [ ] 至少保留一列，最后一列不能被隐藏。
- [ ] 行高切换 compact / default / comfortable 后，单元格高度明显变化。
- [ ] 切换行高后右键菜单、编辑、分页仍可用。

## 回归检查

- [ ] 切换到其他表再回来，表格能正常加载。
- [ ] 切换页码后 pending edits 清空，避免跨页提交错行。
- [ ] 空字符串和 NULL 在表格显示、SQL 预览、导出、导入中区别清楚。
- [ ] 中文、单引号、双引号、逗号、换行、数字都按预期处理。
- [ ] 控制台无新增错误。

## 结果记录

| 项目 | 结果 | 备注 |
| --- | --- | --- |
| 打开表数据页 |  |  |
| 右键基础动作 |  |  |
| 编辑提交/放弃 |  |  |
| 筛选/排序/清除 |  |  |
| 删除确认 |  |  |
| 工具栏/状态栏 |  |  |
| 导出 |  |  |
| 导入 |  |  |
| Stop Loading |  |  |
| Copy As / UUID |  |  |
| 显示设置 |  |  |
| 回归检查 |  |  |

