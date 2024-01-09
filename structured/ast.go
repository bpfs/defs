// AST: 语法树
package bpfsfql

// Node 是所有AST节点都必须实现的接口。
type Node interface {
	TokenLiteral() string // 获取Token的文字内容
}

// Statement 代表FQL中的语句。例如“CREATE FILE...”或“DROP FILE...”。
type Statement interface {
	Node
	statementNode() // 仅用于区分其他节点
}

// Expression 代表FQL中的表达式。
type Expression interface {
	Node
	expressionNode() // 仅用于区分其他节点
}

// ActionStatement 代表FQL中的操作指令，例如CREATE、DROP等。
type ActionStatement struct {
	Token      Token                 // 操作指令的Token，例如CREATE、DROP等
	Name       string                // 操作指令的名称
	Target     Expression            // 命令的目标，如FILE、FILES、EXTENSION等
	Parameters []Expression          // 命令的参数列表，如OWNED_BY、METADATA等
	Lists      []ListExpression      // 新增: 命令的LIST列表，如[file1, file2]
	Conditions []ConditionExpression // 新增: 命令的条件列表，如SIZE < 5000
}

func (as *ActionStatement) statementNode() {}
func (as *ActionStatement) TokenLiteral() string {
	return as.Token.Literal
}

// TargetExpression 代表命令的目标，例如FILE、FILES等。
type TargetExpression struct {
	Token Token  // 目标的Token
	Name  string // 目标的名称，如"FILE"
}

func (te *TargetExpression) expressionNode() {}
func (te *TargetExpression) TokenLiteral() string {
	return te.Token.Literal
}

// AliasExpression 代表FQL中的别名。
type AliasExpression struct {
	Token Token  // 别名的Token
	Name  string // 别名，例如filename、version_number
}

func (ae *AliasExpression) expressionNode() {}
func (ae *AliasExpression) TokenLiteral() string {
	return ae.Token.Literal
}

// ParameterExpression 代表FQL中的参数。
type ParameterExpression struct {
	Token Token      // 参数的Token
	Key   string     // 参数的键，如OWNED_BY、METADATA
	Value Expression // 参数的值，可以是StringLiteral, ListExpression, ConditionExpression等
}

func (pe *ParameterExpression) expressionNode() {}
func (pe *ParameterExpression) TokenLiteral() string {
	return pe.Token.Literal
}

// StringLiteral 代表字符串字面值。
type StringLiteral struct {
	Token Token  // 字符串Token
	Value string // 字符串的具体内容
}

func (sl *StringLiteral) expressionNode() {}
func (sl *StringLiteral) TokenLiteral() string {
	return sl.Token.Literal
}

// ListExpression 代表一个列表。
type ListExpression struct {
	Token    Token        // 列表的Token
	Elements []Expression // 列表中的元素
}

func (le *ListExpression) expressionNode() {}
func (le *ListExpression) TokenLiteral() string {
	return le.Token.Literal
}

// ConditionExpression 代表一个条件表达式。
type ConditionExpression struct {
	Token    Token                // 条件的Token
	Left     Expression           // 条件的左侧表达式
	Operator string               // 条件的操作符，如<、>等
	Right    Expression           // 条件的右侧表达式
	Next     *ConditionExpression // 附加的条件，用于AND或OR连接的多个条件
}

func (ce *ConditionExpression) expressionNode() {}
func (ce *ConditionExpression) TokenLiteral() string {
	return ce.Token.Literal
}

// ListStatement 表示一个列表，例如[file1, file2]
type ListStatement struct {
	Token  Token   // the '[' token
	Values []Token // the elements in the list
}

func (ls *ListStatement) statementNode() {}
func (ls *ListStatement) TokenLiteral() string {
	return ls.Token.Literal
}

// ConditionStatement 表示一个条件语句，例如SIZE < 5000
type ConditionStatement struct {
	Token    Token // the 'WHERE' token
	Operator Token // the '<' token
	Left     Token // the 'SIZE' token
	Right    Token // the '5000' token
}

func (cs *ConditionStatement) statementNode() {}
func (cs *ConditionStatement) TokenLiteral() string {
	return cs.Token.Literal
}

// 您可以根据需要进一步完善和调整AST的结构和定义。
