去中心化文件查询语言（FQL）设计方案。
这个设计方案考虑了文件的所有权、批量操作、标签管理、版本控制、条件语句、插件扩展以及异步操作等多个方面。

==========================================================


处理自定义语言（FQL）设计的词法分析器。该分析器主要用于将用户输入的字符串（一条FQL命令）转化为一组Token。下面，我将尽量概述一下这段代码的核心部分和基本流程。

关键类型和常量
TokenType 用于标记不同类型的Token，如 ACTION、'filename'、PARAM等。
有两个映射表，keywordMap 和 operatorMap，分别用于匹配关键字和操作符，将字符串映射到相应的 TokenType。
Token 结构体用于存储一个词法单元的类型和字面值。
主要函数
1. Lex
Lex 函数是该词法分析器的核心，它的任务是将输入字符串转化为一组Token。
它首先调用 encodeSpaces 函数来处理带引号的字符串中的空格，并且保存了原始字符串和修改过的字符串之间的映射关系。
然后，它使用 strings.FieldsFunc 函数来分割字符串，按空格分割，但保留带引号的部分。
最后，它遍历所有分割出来的部分，并尝试将每个部分转化为一个Token。
2. encodeSpaces 和 decodeSpaces
encodeSpaces 函数会查找所有单引号和双引号之间的字符串，并将这些字符串中的空格替换为 "SPACE"。
decodeSpaces 函数会还原 encodeSpaces 函数中替换过的空格。
3. parseKeyValue, parseList, parseParam, parseDate, parseNumber
这些函数用于处理特定格式的字符串，并尝试将其转化为一个Token。
流程概述
输入字符串的处理：
输入字符串首先通过 encodeSpaces 函数，将字符串中的空格（如果在单引号或双引号之间）进行编码处理。

字符串分割：
处理过的字符串然后被分割成多个部分，每个部分都可能是一个Token。

Token生成：
每个部分都会尝试按照多种规则（关键字、操作符、键值对、列表等）转化为一个Token。

解码处理：
生成的Token列表最后会通过 decodeSpaces 函数，将之前编码过的空格进行还原。

注意事项
未处理的特例：
如果输入字符串中有某个部分不能被识别为任何已定义的Token，Lex 函数会返回一个错误。

列表和日期的处理：
当处理到一个以 "[" 开始的部分时，会将其视为一个列表的开始，并开始收集后续的部分直到遇到 "]"，然后将收集到的部分尝试转化为一个列表Token或日期Token。

参数的处理：
如果某个部分符合 KEY=VALUE 格式，会尝试将其处理为一个参数Token。

结论
这个词法分析器的目标是将FQL语句转化为Token，可以处理多种格式和类型的Token，但对于非法的输入，它会返回错误。这是构建一个解释器或编译器的第一步，下一步通常是语法分析，将Token转化为抽象语法树。

==========================================================

# 文件定义语言（FDL）
1. 创建文件
    CREATE FILE 'filename' OWNED_BY='did' CUSTOM_NAME=custom_name METADATA="key1:value1,key2:value2"
    - 'filename': 要创建的文件的名称。
    - OWNER: 文件的拥有者的DID。
    - CUSTOM_NAME: 可选。为文件设定的自定义名称。
    - METADATA: 可选。为文件附加的额外信息或属性。

2. 删除文件（需要特别注意权限）
    DROP FILE 'filename' AUTHORIZED BY 'did'
    - 'filename': 要删除的文件的名称。
    - AUTHORIZED BY: 执行操作的DID，必须有权限。

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

# 版本控制
1. 检出特定版本
    CHECKOUT FILE 'filename' VERSION version_number AUTHORIZED BY 'did'
    - 'filename': 要检出的文件的名称。
    - VERSION: 文件的特定版本号。
    - AUTHORIZED BY: 执行操作的DID，必须有权限。

# 插件或扩展支持
1. 使用扩展
    USE EXTENSION 'extension_name' PARAMETERS="key1:value1,key2:value2"
    - extension_name: 要使用的扩展或插件名称。
    - PARAMETERS: 扩展或插件所需的参数。

# 异步操作支持
1. 异步创建文件
    ASYNC CREATE FILE 'filename' OWNED_BY='did'
    - 'filename': 要异步创建的文件的名称。
    - OWNED_BY: 文件的拥有者的DID。




# 文件定义语言（FDL）
1. 创建文件
CREATE FILE 'filename' OWNED_BY='did' CUSTOM_NAME=custom_name METADATA="key1:value1,key2:value2"

2. 删除文件（需要特别注意权限）
DROP FILE 'filename' AUTHORIZED BY 'did'

# 文件操作语言（FOL）
1. 更新文件内容
UPDATE FILE 'filename' SET CONTENT='new content' WHERE SIZE < 5000 AND LAST_MODIFIED > '2023-01-01' AUTHORIZED BY 'did'

2. 读取文件内容
SELECT CONTENT FROM FILE 'filename' AUTHORIZED BY 'did'

3. 批量文件转移
BULK TRANSFER FILES [file1, file2, ...] FROM did_from TO did_to

4. 添加标签
ADD TAGS TO FILE 'filename' TAGS [tag1, tag2, ...] AUTHORIZED BY 'did'

5. 列出所有文件（可加筛选条件）
SHOW FILES WHERE TAGS INCLUDE [tag1, tag2, ...] AND OWNED_BY='did'

# 文件控制语言（FCL）
1. 资产转移
TRANSFER FILE 'filename' TO did AUTHORIZED BY 'did'

2. 共有产权设置
SET CO_OWNERS FOR FILE 'filename' CO_OWNERS=[did1, did2, ...] AUTHORIZED BY 'did'

3. 资产共享
SHARE FILE 'filename' WITH 'did' AUTHORIZED BY 'did'

版本控制
1. 检出特定版本
CHECKOUT FILE 'filename' VERSION version_number AUTHORIZED BY 'did'

插件或扩展支持
1. 使用扩展
USE EXTENSION 'extension_name' PARAMETERS="key1:value1,key2:value2"

异步操作支持
1. 异步创建文件
ASYNC CREATE FILE 'filename' OWNED_BY='did'





1. CREATE FILE 'filename' OWNED_BY='did' CUSTOM_NAME='custom_name' METADATA='key1:value1, key2:value2'
2. DROP FILE 'filename' AUTHORIZED BY 'did'
3. UPDATE FILE 'filename' SET CONTENT='new content' WHERE SIZE < 5000 AND LAST_MODIFIED > '2023-01-01' AUTHORIZED BY 'did'
4. SELECT CONTENT FROM FILE 'filename' AUTHORIZED BY 'did'
5. BULK TRANSFER FILES ['file1', 'file2', ...] FROM 'did_from' TO 'did_to'
6. ADD TAGS TO FILE 'filename' TAGS ['tag1', 'tag2', ...] AUTHORIZED BY 'did'
7. SHOW FILES WHERE TAGS INCLUDE ['tag1', 'tag2', ...] AND OWNED_BY='did'
8. TRANSFER FILE 'filename' TO 'did' AUTHORIZED BY 'did'
9. SET CO_OWNERS FOR FILE 'filename' CO_OWNERS=['did1', 'did2', ...] AUTHORIZED BY 'did'
10. SHARE FILE 'filename' WITH 'did' AUTHORIZED BY 'did'
11. CHECKOUT FILE 'filename' VERSION 'version_number' AUTHORIZED BY 'did'
12. USE EXTENSION 'extension_name' PARAMETERS='key1:value1, key2:value2'
13. ASYNC CREATE FILE 'filename' OWNED_BY='did'




# 第二版 语句

创建文件
语法：CREATE FILE 'filepath' OWNER 'did' CUSTOM_NAME 'custom_name' METADATA 'key1:value1 key2:value2'

删除文件
语法：DROP FILE 'filehash' AUTHORIZED BY 'did'
DELETE 用于删除文件内的字段

更新文件内容
语法：UPDATE FILE 'filehash' SET CONTENT 'new content' WHERE SIZE < 5000 AND LAST_MODIFIED > '2023-01-01' AUTHORIZED BY 'did'

查询文件内容
语法：SELECT CONTENT FROM FILE 'filehash' AUTHORIZED BY 'did'

批量转移文件
语法：BULK TRANSFER FILES 'filehash1' 'filehash2' ... FROM 'did_from' TO 'did_to'

给文件添加标签
语法：ALTER FILE 'filehash' ADD TAGS 'tag1' 'tag2' ... AUTHORIZED BY 'did'

显示具有特定标签和所有者的文件
语法：SELECT FILES WHERE TAGS INCLUDE 'tag1' 'tag2' ... AND OWNER 'did'

转移文件
语法：TRANSFER FILE 'filehash' TO 'did' AUTHORIZED BY 'did'

设置文件的共有者
语法：ALTER FILE 'filehash' SET CO_OWNERS 'did1' 'did2' ... AUTHORIZED BY 'did'

分享文件
语法：SHARE FILE 'filehash' WITH 'did' AUTHORIZED BY 'did'

文件版本检出
语法：CHECKOUT FILE 'filehash' VERSION 'version_number' AUTHORIZED BY 'did'

使用扩展
语法：USE EXTENSION 'extension_name' PARAMETERS 'key1:value1 key2:value2'

异步创建文件
语法：ASYNC CREATE FILE 'filepath' OWNER 'did'