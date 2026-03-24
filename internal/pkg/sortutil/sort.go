package sortutil

import "fmt"

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
