package sortutil

import "fmt"

// ReorderSiblings 把 movedID 从 siblings 中抽出，插到 beforeID 之前；
// beforeID == 0 或未在 siblings 中找到时，追加到末尾。
// 返回重排后的切片；若 movedID 未在 siblings 中找到，原样返回。
func ReorderSiblings[T any](siblings []T, movedID, beforeID int64, getID func(T) int64) []T {
	var moved T
	var foundMoved bool
	rest := make([]T, 0, len(siblings))
	for _, item := range siblings {
		if getID(item) == movedID {
			moved = item
			foundMoved = true
			continue
		}
		rest = append(rest, item)
	}
	if !foundMoved {
		return siblings
	}

	if beforeID == 0 {
		return append(rest, moved)
	}
	result := make([]T, 0, len(siblings))
	inserted := false
	for _, item := range rest {
		if !inserted && getID(item) == beforeID {
			result = append(result, moved)
			inserted = true
		}
		result = append(result, item)
	}
	if !inserted {
		result = append(result, moved)
	}
	return result
}

// MoveItem 通用排序移动逻辑（up/down/top）
func MoveItem[T any](id int64, direction string, items []T,
	getID func(T) int64, getOrder func(T) int, updateOrder func(int64, int) error,
) error {
	idx := -1
	for i, item := range items {
		if getID(item) == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("item not found")
	}

	switch direction {
	case "up":
		if idx == 0 {
			return nil
		}
		prevOrder := getOrder(items[idx-1])
		curOrder := getOrder(items[idx])
		if prevOrder == curOrder {
			curOrder = prevOrder + 1
		}
		if err := updateOrder(getID(items[idx]), prevOrder); err != nil {
			return err
		}
		return updateOrder(getID(items[idx-1]), curOrder)
	case "down":
		if idx == len(items)-1 {
			return nil
		}
		nextOrder := getOrder(items[idx+1])
		curOrder := getOrder(items[idx])
		if nextOrder == curOrder {
			nextOrder = curOrder + 1
		}
		if err := updateOrder(getID(items[idx]), nextOrder); err != nil {
			return err
		}
		return updateOrder(getID(items[idx+1]), curOrder)
	case "top":
		if idx == 0 {
			return nil
		}
		firstOrder := getOrder(items[0])
		return updateOrder(id, firstOrder-1)
	default:
		return fmt.Errorf("invalid direction: %s", direction)
	}
}
