package helper

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// extractContextSizeFromModel 从模型名称中提取上下文大小（单位：k）
// 示例：gpt-4-100k -> 100, claude-[200K] -> 200, model(128k) -> 128
// 如果模型名有多个匹配，返回最大值
func extractContextSizeFromModel(modelName string) int {
	if modelName == "" {
		return 0
	}

	// 正则：匹配数字+k/K
	re := regexp.MustCompile(`(\d+)[kK]`)
	matches := re.FindAllStringSubmatch(modelName, -1)

	if len(matches) == 0 {
		return 0
	}

	// 如果有多个匹配，取最大值
	maxSize := 0
	for _, match := range matches {
		if len(match) > 1 {
			size, err := strconv.Atoi(match[1])
			if err == nil && size > maxSize {
				maxSize = size
			}
		}
	}

	return maxSize
}

// ValidateContextLimit 验证输入token数量是否超过模型上下文限制
// 返回错误信息（如果超限）
func ValidateContextLimit(modelName string, inputTokens int) error {
	contextSizeK := extractContextSizeFromModel(modelName)

	// 没有上下文标识，不做限制
	if contextSizeK == 0 {
		return nil
	}

	// 计算限制：K值 * 1000
	limit := contextSizeK * 1000

	// 检查是否超限
	if inputTokens > limit {
		// 判断是否为 Claude 模型
		isClaudeModel := strings.Contains(strings.ToLower(modelName), "claude")

		if isClaudeModel {
			// Claude 模型：简洁提示
			return fmt.Errorf("Claude 模型的最大输入上限是 %dk", contextSizeK)
		}

		// 其他模型：详细提示
		limitWan := float64(contextSizeK) / 10.0
		inputWan := float64(inputTokens) / 10000.0

		return fmt.Errorf("小熊猫温馨提示喔：当前请求的输入字数是%d（约%.1f万），本模型的最大输入是%dk（约%.1f万），输入超字数啦~可以总结隐藏下~也可以换更大的上下文模型，比如：如果当前是100k，就换200k。另外，gemini模型名上没有k的模型，都是满上下文100w字喔，爱你！",
			inputTokens, inputWan, contextSizeK, limitWan)
	}

	return nil
}
