package defs

/**

# 文件定义语言（FDL）

1. 创建文件
    CREATE FILE filename OWNED_BY=did CUSTOM_NAME=custom_name METADATA="key1:value1,key2:value2"
    - filename: 要创建的文件的名称。
    - OWNED_BY: 文件的拥有者的DID。
    - CUSTOM_NAME: 可选。为文件设定的自定义名称。
    - METADATA: 可选。为文件附加的额外信息或属性。

2. 删除文件（需要特别注意权限）
    DROP FILE 'filename' AUTHORIZED BY 'did'
    - 'filename': 要删除的文件的名称。
    - AUTHORIZED BY: 执行操作的DID，必须有权限。

*/
