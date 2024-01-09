package defs

/**

# 文件操作语言（FOL）

1. 更新文件内容
    UPDATE FILE 'filename' SET CONTENT='new content' WHERE SIZE < 5000 AND LAST_MODIFIED > '2023-01-01' AUTHORIZED BY 'did'
    - 'filename': 要更新的文件名称。
    - CONTENT: 更新后的文件内容。
    - SIZE 和 LAST_MODIFIED: 文件筛选条件。
    - AUTHORIZED BY: 执行操作的DID，必须有权限。

2. 读取文件内容
    SELECT CONTENT FROM FILE 'filename' AUTHORIZED BY 'did'
    - 'filename': 要读取的文件的名称。
    - AUTHORIZED BY: 执行操作的DID，必须有权限。

3. 批量文件转移
    BULK TRANSFER FILES [file1, file2, ...] FROM did_from TO did_to
    - FILES: 要转移的文件列表。
    - did_from 和 did_to: 资产的转移方和接收方的DID。

4. 添加标签
    ADD TAGS TO FILE 'filename' TAGS [tag1, tag2, ...] AUTHORIZED BY 'did'
    - 'filename': 要添加标签的文件的名称。
    - TAGS: 要添加的标签列表。
    - AUTHORIZED BY: 执行操作的DID，必须有权限。

5. 列出所有文件（可加筛选条件）
    SHOW FILES WHERE TAGS INCLUDE [tag1, tag2, ...] AND OWNED_BY='did'
    - TAGS: 文件的标签筛选条件。
    - OWNED_BY: 文件的拥有者DID筛选条件。

*/
