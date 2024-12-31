// 实现了一个数据栈，用于脚本执行过程中的数据存储。

package script

import (
	"encoding/hex"
	"fmt"
)

// asBool 获取字节数组的布尔值。
func asBool(t []byte) bool {
	for i := range t {
		if t[i] != 0 {
			// Negative 0 is also considered false.
			if i == len(t)-1 && t[i] == 0x80 {
				return false
			}
			return true
		}
	}
	return false
}

// fromBool 将布尔值转换为适当的字节数组。
func fromBool(v bool) []byte {
	if v {
		return []byte{1}
	}
	return nil
}

// stack 表示与脚本一起使用的不可变对象的堆栈。
// 对象可以共享，因此在使用中如果要更改某个值，则必须首先对其进行深度复制，以避免更改堆栈上的其他值。
type stack struct {
	stk               [][]byte
	verifyMinimalData bool
}

// Depth 返回堆栈上的项目数。
func (s *stack) Depth() int32 {
	return int32(len(s.stk))
}

// PushByteArray 将给定的返回数组添加到堆栈顶部。
//
// 堆栈转换: [... x1 x2] -> [... x1 x2 data]
func (s *stack) PushByteArray(so []byte) {
	s.stk = append(s.stk, so)
}

// PushInt 将提供的 scriptNum 转换为合适的字节数组，然后将其推入堆栈顶部。
//
// 堆栈转换: [... x1 x2] -> [... x1 x2 int]
func (s *stack) PushInt(val scriptNum) {
	s.PushByteArray(val.Bytes())
}

// PushBool 将提供的布尔值转换为合适的字节数组，然后将其推入堆栈顶部。
//
// 堆栈转换: [... x1 x2] -> [... x1 x2 bool]
func (s *stack) PushBool(val bool) {
	s.PushByteArray(fromBool(val))
}

// PopByteArray 将值从堆栈顶部弹出并返回。
//
// 堆栈转换: [... x1 x2 x3] -> [... x1 x2]
func (s *stack) PopByteArray() ([]byte, error) {
	return s.nipN(0)
}

// PopInt 将值从堆栈顶部弹出，将其转换为脚本编号，然后返回它。
// 转换为脚本 num 的行为强制执行对解释为数字的数据强加的共识规则。
//
// 堆栈转换: [... x1 x2 x3] -> [... x1 x2]
func (s *stack) PopInt() (scriptNum, error) {
	so, err := s.PopByteArray()
	if err != nil {
		return 0, err
	}

	return MakeScriptNum(so, s.verifyMinimalData, maxScriptNumLen)
}

// PopBool 将值从堆栈顶部弹出，将其转换为布尔值，然后返回它。
//
// 堆栈转换: [... x1 x2 x3] -> [... x1 x2]
func (s *stack) PopBool() (bool, error) {
	so, err := s.PopByteArray()
	if err != nil {
		return false, err
	}

	return asBool(so), nil
}

// PeekByteArray 返回堆栈中的第 N 个项目而不删除它。
func (s *stack) PeekByteArray(idx int32) ([]byte, error) {
	sz := int32(len(s.stk))
	if idx < 0 || idx >= sz {
		return nil, fmt.Errorf("index %d is invalid for stack size %d", idx, sz)
	}

	return s.stk[sz-idx-1], nil
}

// PeekInt 将堆栈中的第 N 个项目作为脚本编号返回，而不将其删除。
// 转换为脚本 num 的行为强制执行对解释为数字的数据强加的共识规则。
func (s *stack) PeekInt(idx int32) (scriptNum, error) {
	so, err := s.PeekByteArray(idx)
	if err != nil {
		return 0, err
	}

	return MakeScriptNum(so, s.verifyMinimalData, maxScriptNumLen)
}

// PeekBool 将堆栈中的第 N 个项目作为 bool 返回，而不将其删除。
func (s *stack) PeekBool(idx int32) (bool, error) {
	so, err := s.PeekByteArray(idx)
	if err != nil {
		return false, err
	}

	return asBool(so), nil
}

// nipN 是一个内部函数，用于删除堆栈中的第 n 项并返回它。
//
// 堆栈转换:
// nipN(0): [... x1 x2 x3] -> [... x1 x2]
// nipN(1): [... x1 x2 x3] -> [... x1 x3]
// nipN(2): [... x1 x2 x3] -> [... x2 x3]
func (s *stack) nipN(idx int32) ([]byte, error) {
	sz := int32(len(s.stk))
	if idx < 0 || idx > sz-1 {
		return nil, fmt.Errorf("index %d is invalid for stack size %d", idx, sz)
	}

	so := s.stk[sz-idx-1]
	if idx == 0 {
		s.stk = s.stk[:sz-1]
	} else if idx == sz-1 {
		s1 := make([][]byte, sz-1)
		copy(s1, s.stk[1:])
		s.stk = s1
	} else {
		s1 := s.stk[sz-idx : sz]
		s.stk = s.stk[:sz-idx-1]
		s.stk = append(s.stk, s1...)
	}
	return so, nil
}

// NipN 移除堆栈中的第 N 个对象
//
// 堆栈转换:
// NipN(0): [... x1 x2 x3] -> [... x1 x2]
// NipN(1): [... x1 x2 x3] -> [... x1 x3]
// NipN(2): [... x1 x2 x3] -> [... x2 x3]
func (s *stack) NipN(idx int32) error {
	_, err := s.nipN(idx)
	return err
}

// Tuck 复制堆栈顶部的项目并将其插入到顶部第二个项目之前。
//
// 堆栈转换: [... x1 x2] -> [... x2 x1 x2]
func (s *stack) Tuck() error {
	so2, err := s.PopByteArray()
	if err != nil {
		return err
	}
	so1, err := s.PopByteArray()
	if err != nil {
		return err
	}
	s.PushByteArray(so2) // stack [... x2]
	s.PushByteArray(so1) // stack [... x2 x1]
	s.PushByteArray(so2) // stack [... x2 x1 x2]

	return nil
}

// DropN 从堆栈中删除前 N 个项目。
//
// 堆栈转换:
// DropN(1): [... x1 x2] -> [... x1]
// DropN(2): [... x1 x2] -> [...]
func (s *stack) DropN(n int32) error {
	if n < 1 {
		return fmt.Errorf("attempt to drop %d items from stack", n)
	}

	for ; n > 0; n-- {
		_, err := s.PopByteArray()
		if err != nil {
			return err
		}
	}
	return nil
}

// DupN 复制堆栈中前 N 个项目。
//
// 堆栈转换:
// DupN(1): [... x1 x2] -> [... x1 x2 x2]
// DupN(2): [... x1 x2] -> [... x1 x2 x1 x2]
func (s *stack) DupN(n int32) error {
	if n < 1 {
		return fmt.Errorf("attempt to dup %d stack items", n)
	}

	// Iteratively duplicate the value n-1 down the stack n times.
	// This leaves an in-order duplicate of the top n items on the stack.
	for i := n; i > 0; i-- {
		so, err := s.PeekByteArray(n - 1)
		if err != nil {
			return err
		}
		s.PushByteArray(so)
	}
	return nil
}

// RotN 将堆栈顶部的 3N 项向左旋转 N 次。
//
// 堆栈转换:
// RotN(1): [... x1 x2 x3] -> [... x2 x3 x1]
// RotN(2): [... x1 x2 x3 x4 x5 x6] -> [... x3 x4 x5 x6 x1 x2]
func (s *stack) RotN(n int32) error {
	if n < 1 {
		return fmt.Errorf("attempt to rotate %d stack items", n)
	}

	// Nip the 3n-1th item from the stack to the top n times to rotate
	// them up to the head of the stack.
	entry := 3*n - 1
	for i := n; i > 0; i-- {
		so, err := s.nipN(entry)
		if err != nil {
			return err
		}

		s.PushByteArray(so)
	}
	return nil
}

// SwapN 将堆栈顶部的 N 项与其下面的项交换。
//
// 堆栈转换:
// SwapN(1): [... x1 x2] -> [... x2 x1]
// SwapN(2): [... x1 x2 x3 x4] -> [... x3 x4 x1 x2]
func (s *stack) SwapN(n int32) error {
	if n < 1 {
		return fmt.Errorf("attempt to swap %d stack items", n)
	}

	entry := 2*n - 1
	for i := n; i > 0; i-- {
		// Swap 2n-1th entry to top.
		so, err := s.nipN(entry)
		if err != nil {
			return err
		}

		s.PushByteArray(so)
	}
	return nil
}

// OverN 将 N 个项目复制回堆栈顶部。
//
// 堆栈转换:
// OverN(1): [... x1 x2 x3] -> [... x1 x2 x3 x2]
// OverN(2): [... x1 x2 x3 x4] -> [... x1 x2 x3 x4 x1 x2]
func (s *stack) OverN(n int32) error {
	if n < 1 {
		return fmt.Errorf("attempt to perform over on %d stack items", n)
	}

	// Copy 2n-1th entry to top of the stack.
	entry := 2*n - 1
	for ; n > 0; n-- {
		so, err := s.PeekByteArray(entry)
		if err != nil {
			return err
		}
		s.PushByteArray(so)
	}

	return nil
}

// PickN 将 N 个项目复制回堆栈顶部。
//
// 堆栈转换:
// PickN(0): [x1 x2 x3] -> [x1 x2 x3 x3]
// PickN(1): [x1 x2 x3] -> [x1 x2 x3 x2]
// PickN(2): [x1 x2 x3] -> [x1 x2 x3 x1]
func (s *stack) PickN(n int32) error {
	so, err := s.PeekByteArray(n)
	if err != nil {
		return err
	}
	s.PushByteArray(so)

	return nil
}

// RollN 将堆栈中的 N 个项目移回到顶部。
//
// 堆栈转换:
// RollN(0): [x1 x2 x3] -> [x1 x2 x3]
// RollN(1): [x1 x2 x3] -> [x1 x3 x2]
// RollN(2): [x1 x2 x3] -> [x2 x3 x1]
func (s *stack) RollN(n int32) error {
	so, err := s.nipN(n)
	if err != nil {
		return err
	}

	s.PushByteArray(so)

	return nil
}

// String 以可读格式返回堆栈。
func (s *stack) String() string {
	var result string
	for _, stack := range s.stk {
		if len(stack) == 0 {
			result += "00000000  <empty>\n"
		}
		result += hex.Dump(stack)
	}

	return result
}
