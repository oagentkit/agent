package roundguard

import "github.com/oekit/pkg/errors"

// ErrInvalidBudget 预算声明非法（负值上限 / 空工具标识 / 未知处置口径）。
//
// 处置：**修声明**（调用方配置错，重试无意义）。
var ErrInvalidBudget = errors.New("回合预算声明非法")
