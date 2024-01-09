package defs

/**

# 文件控制语言（FCL）

1. 资产转移
    TRANSFER FILE 'filename' TO did AUTHORIZED BY 'did'
    - 'filename': 要转移的文件的名称。
    - TO: 接收资产的DID。
    - AUTHORIZED BY: 执行操作的DID，必须有权限。

2. 共有产权设置
    SET CO_OWNERS FOR FILE 'filename' CO_OWNERS=[did1, did2, ...] AUTHORIZED BY 'did'
    - 'filename': 要设置共有产权的文件的名称。
    - CO_OWNERS: 共有者的DID列表。
    - AUTHORIZED BY: 执行操作的DID，必须有权限。

3. 资产共享
    SHARE FILE 'filename' WITH 'did' AUTHORIZED BY 'did'
    - 'filename': 要共享的文件的名称。
    - WITH: 共享资产的目标DID。
    - AUTHORIZED BY: 执行操作的DID，必须有权限。

*/
