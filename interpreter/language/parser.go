package language

import (
	"fmt"
)

// Parser represent the interpreter parser
type Parser struct {
	l         *Lexer
	curToken  Token
	peekToken Token
	errors    []string

	prefixParseFns map[TokenType]prefixParseFn
	infixParseFns  map[TokenType]infixParseFn
}

type (
	prefixParseFn func() Expression
	infixParseFn  func(Expression) Expression
)

const (
	_ int = iota
	precedenceValueLowset
	precedenceValueOR                // OR
	precedenceValueAND               // AND
	precedenceValueNOT               // NOT
	precedenceValueEqualComparators  // = <>
	precedenceValueBetweenComparator // BETWEEN
	precedenceValueComparators       // < <= > >=
	precedenceValueCall              // myFunction(X)
)

var precedences = map[TokenType]int{
	EQ:      precedenceValueEqualComparators,
	NotEQ:   precedenceValueEqualComparators,
	BETWEEN: precedenceValueBetweenComparator,
	LT:      precedenceValueComparators,
	GT:      precedenceValueComparators,
	LTE:     precedenceValueComparators,
	GTE:     precedenceValueComparators,
	AND:     precedenceValueAND,
	OR:      precedenceValueOR,
	LPAREN:  precedenceValueCall,
}

// NewParser creates a new parser
func NewParser(l *Lexer) *Parser {
	p := &Parser{
		l:      l,
		errors: []string{},
	}

	p.prefixParseFns = map[TokenType]prefixParseFn{}
	p.registerPrefix(IDENT, p.parseIdentifier)
	p.registerPrefix(NOT, p.parsePrefixExpression)
	p.registerPrefix(LPAREN, p.parseGroupedExpression)

	p.infixParseFns = make(map[TokenType]infixParseFn)
	p.registerInfix(EQ, p.parseInfixExpression)
	p.registerInfix(NotEQ, p.parseInfixExpression)
	p.registerInfix(BETWEEN, p.parseBetweenExpression)
	p.registerInfix(LT, p.parseInfixExpression)
	p.registerInfix(GT, p.parseInfixExpression)
	p.registerInfix(LTE, p.parseInfixExpression)
	p.registerInfix(GTE, p.parseInfixExpression)
	p.registerInfix(AND, p.parseInfixExpression)
	p.registerInfix(OR, p.parseInfixExpression)
	p.registerInfix(LPAREN, p.parseCallExpression)

	// Read two tokens, so curToken and peekToken are both set
	p.nextToken()
	p.nextToken()

	return p
}

func (p *Parser) parseIdentifier() Expression {
	return &Identifier{Token: p.curToken, Value: p.curToken.Literal}
}

// Errors returns the errors found while parsing
func (p *Parser) Errors() []string {
	return p.errors
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.l.NextToken()
}

// ParseDynamoExpression parse the given dynamodb expression
func (p *Parser) ParseDynamoExpression() *DynamoExpression {
	program := &DynamoExpression{}

	for p.curToken.Type != EOF {
		stmt := p.parseExpressionStatement()
		if stmt != nil {
			program.Statement = stmt
		}

		p.nextToken()
	}

	return program
}

func (p *Parser) parseExpressionStatement() *ExpressionStatement {
	stmt := &ExpressionStatement{Token: p.curToken}
	stmt.Expression = p.parseExpression(precedenceValueLowset)

	if p.peekTokenIs(EOF) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseGroupedExpression() Expression {
	p.nextToken()
	exp := p.parseExpression(precedenceValueLowset)

	if !p.expectPeek(RPAREN) {
		return nil
	}

	return exp
}

func (p *Parser) noPrefixParseFnError(t TokenType) {
	msg := fmt.Sprintf("no prefix parse function for %s found", t)
	p.errors = append(p.errors, msg)
}

func (p *Parser) parseExpression(precedence int) Expression {
	prefix := p.prefixParseFns[p.curToken.Type]

	if prefix == nil {
		p.noPrefixParseFnError(p.curToken.Type)

		return nil
	}

	leftExp := prefix()

	for !p.peekTokenIs(EOF) && precedence < p.peekPrecedence() {
		infix := p.infixParseFns[p.peekToken.Type]
		if infix == nil {
			return leftExp
		}

		p.nextToken()

		leftExp = infix(leftExp)
	}

	return leftExp
}

func (p *Parser) parsePrefixExpression() Expression {
	expression := &PrefixExpression{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
	}

	p.nextToken()
	expression.Right = p.parseExpression(precedenceValueNOT)

	return expression
}

func (p *Parser) parseInfixExpression(left Expression) Expression {
	expression := &InfixExpression{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
		Left:     left,
	}

	precedence := precedenceValueLowset
	if p, ok := precedences[p.curToken.Type]; ok {
		precedence = p
	}

	p.nextToken()
	expression.Right = p.parseExpression(precedence)

	return expression
}

func (p *Parser) parseCallExpression(function Expression) Expression {
	exp := &CallExpression{Token: p.curToken, Function: function}

	exp.Arguments = p.parseCallArguments()

	return exp
}

func (p *Parser) parseBetweenExpression(left Expression) Expression {
	expression := &BetweenExpression{
		Token: p.curToken,
		Left:  left,
		Range: [2]Expression{},
	}

	p.nextToken()
	expression.Range[0] = p.parseIdentifier()

	if !p.expectPeek(AND) {
		return nil
	}

	p.nextToken()
	expression.Range[1] = p.parseIdentifier()

	return expression
}

func (p *Parser) parseCallArguments() []Expression {
	args := []Expression{}

	if p.peekTokenIs(RPAREN) {
		p.nextToken()
		return args
	}

	p.nextToken()
	args = append(args, p.parseExpression(precedenceValueLowset))

	for p.peekTokenIs(COMMA) {
		p.nextToken()
		p.nextToken()
		args = append(args, p.parseExpression(precedenceValueLowset))
	}

	if !p.expectPeek(RPAREN) {
		return nil
	}

	return args
}

// helpers

func (p *Parser) peekTokenIs(t TokenType) bool {
	return p.peekToken.Type == t
}

func (p *Parser) expectPeek(t TokenType) bool {
	if !p.peekTokenIs(t) {
		p.peekError(t)

		return false
	}

	p.nextToken()

	return true
}

func (p *Parser) peekError(t TokenType) {
	msg := fmt.Sprintf("expected next token to be %s, got %s instead",
		t, p.peekToken.Type)
	p.errors = append(p.errors, msg)
}

func (p *Parser) registerPrefix(tokenType TokenType, fn prefixParseFn) {
	p.prefixParseFns[tokenType] = fn
}

func (p *Parser) registerInfix(tokenType TokenType, fn infixParseFn) {
	p.infixParseFns[tokenType] = fn
}

func (p *Parser) peekPrecedence() int {
	if p, ok := precedences[p.peekToken.Type]; ok {
		return p
	}

	return precedenceValueLowset
}
