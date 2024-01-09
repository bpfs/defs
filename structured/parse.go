// Parse: 语法分析
package bpfsfql

import "strings"

// Parser 结构体
type Parser struct {
	tokens  []Token
	current int
}

// NewParser 创建一个新的Parser实例
func NewParser(tokens []Token) *Parser {
	return &Parser{tokens: tokens}
}

// ParseStatement 解析一个语句
func (p *Parser) ParseStatement() Statement {
	switch {
	// case p.currentToken().Type == CREATE:
	// return p.parseCreateStatement()
	case p.currentToken().Type == ACTION:
		return p.parseActionStatement()
	default:
		// Handle other cases or throw error
		return nil
	}
}

func (p *Parser) parseCreateStatement() *ActionStatement {
	stmt := &ActionStatement{Token: p.currentToken()}
	p.advance() // Move past CREATE
	stmt.Target = p.parseTargetExpression()

	for p.currentToken().Type == PARAM {
		param := p.parseParameterExpression()
		stmt.Parameters = append(stmt.Parameters, param)
		p.advance()
	}

	return stmt
}

// parseActionStatement 解析一个动作语句，例如 CREATE, DROP 等
func (p *Parser) parseActionStatement() *ActionStatement {
	stmt := &ActionStatement{Token: p.currentToken()}
	stmt.Name = p.currentToken().Literal
	p.advance() // Move to next token

	stmt.Target = p.parseTargetExpression()

	// 根据语法结构，循环解析参数、列表和条件
	for {
		switch {
		case p.currentToken().Type == PARAM:
			param := p.parseParameterExpression()
			stmt.Parameters = append(stmt.Parameters, param)
		case p.currentToken().Type == LIST:
			list := p.parseListStatement()
			stmt.Lists = append(stmt.Lists, *list)
		case p.currentToken().Type == OPERATOR:
			cond := p.parseConditionStatement()
			stmt.Conditions = append(stmt.Conditions, *cond)
		default:
			return stmt // 如果遇到其他Token类型，则返回stmt
		}
		p.advance()
	}
}

// parseTargetExpression 解析目标表达式，例如 FILE
func (p *Parser) parseTargetExpression() Expression {
	expr := &TargetExpression{
		Token: p.currentToken(),
		Name:  p.currentToken().Literal,
	}
	p.advance() // Move to next token
	return expr
}

// parseParameterExpression 解析一个参数表达式
func (p *Parser) parseParameterExpression() Expression {
	param := &ParameterExpression{
		Token: p.currentToken(),
		Key:   p.currentToken().Literal,
	}
	p.advance()
	param.Value = p.parseExpression()
	return param
}

// parseListStatement 解析一个列表
func (p *Parser) parseListStatement() *ListExpression {
	list := &ListExpression{Token: p.currentToken()}
	p.advance()
	for p.currentToken().Type != OPTION {
		elem := p.parseExpression()
		list.Elements = append(list.Elements, elem)
		p.advance()
	}
	return list
}

// parseConditionStatement 解析一个条件语句
func (p *Parser) parseConditionStatement() *ConditionExpression {
	cond := &ConditionExpression{
		Token:    p.currentToken(),
		Left:     p.parseExpression(),
		Operator: p.peekToken().Literal,
	}
	p.advance()
	p.advance()
	cond.Right = p.parseExpression()
	return cond
}

// parseExpression 解析基本的表达式
func (p *Parser) parseExpression() Expression {
	switch {
	// case p.currentToken().Type == STRING_LITERAL:
	// 	return &StringLiteral{
	// 		Token: p.currentToken(),
	// 		Value: p.currentToken().Literal,
	// 	}
	default:
		// Handle other expression types or throw error
		return nil
	}
}

func parseMetadata(data string) map[string]string {
	metadata := make(map[string]string)

	// 移除双引号
	data = strings.Trim(data, "\"")

	pairs := strings.Split(data, ",")
	for _, pair := range pairs {
		kv := strings.Split(pair, ":")
		if len(kv) == 2 {
			metadata[kv[0]] = kv[1]
		}
	}

	return metadata
}

func (p *Parser) currentToken() Token {
	return p.tokens[p.current]
}

func (p *Parser) peekToken() Token {
	if p.current+1 < len(p.tokens) {
		return p.tokens[p.current+1]
	}
	return Token{} // Return an empty token if out of bounds
}

func (p *Parser) advance() {
	p.current++
}
