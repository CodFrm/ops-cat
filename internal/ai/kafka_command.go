package ai

import (
	"fmt"
	"strings"
)

// kafkaClusterCommand 将 kafka_cluster 工具的 operation 映射为策略命令字符串。
func kafkaClusterCommand(operation string) (string, error) {
	switch operation {
	case "overview":
		return "cluster.read *", nil
	case "brokers", "list_brokers":
		return "broker.read *", nil
	case "get_broker_config":
		return "broker.config.read *", nil
	case "list_cluster_configs":
		return "cluster.config.read *", nil
	default:
		return "", fmt.Errorf("unsupported kafka_cluster operation: %s", operation)
	}
}

func kafkaTopicCommand(operation, topic string) (string, error) {
	switch operation {
	case "list":
		return "topic.list *", nil
	case "get", "describe":
		topic = strings.TrimSpace(topic)
		if topic == "" {
			return "", fmt.Errorf("topic is required for kafka_topic %s", operation)
		}
		return "topic.read " + topic, nil
	case "create":
		topic = strings.TrimSpace(topic)
		if topic == "" {
			return "", fmt.Errorf("topic is required for kafka_topic %s", operation)
		}
		return "topic.create " + topic, nil
	case "delete":
		topic = strings.TrimSpace(topic)
		if topic == "" {
			return "", fmt.Errorf("topic is required for kafka_topic %s", operation)
		}
		return "topic.delete " + topic, nil
	case "update_config":
		topic = strings.TrimSpace(topic)
		if topic == "" {
			return "", fmt.Errorf("topic is required for kafka_topic %s", operation)
		}
		return "topic.config.write " + topic, nil
	case "increase_partitions":
		topic = strings.TrimSpace(topic)
		if topic == "" {
			return "", fmt.Errorf("topic is required for kafka_topic %s", operation)
		}
		return "topic.partitions.write " + topic, nil
	case "delete_records":
		topic = strings.TrimSpace(topic)
		if topic == "" {
			return "", fmt.Errorf("topic is required for kafka_topic %s", operation)
		}
		return "topic.records.delete " + topic, nil
	default:
		return "", fmt.Errorf("unsupported kafka_topic operation: %s", operation)
	}
}

func kafkaConsumerGroupCommand(operation, group string) (string, error) {
	switch operation {
	case "list":
		return "consumer_group.list *", nil
	case "get", "describe":
		group = strings.TrimSpace(group)
		if group == "" {
			return "", fmt.Errorf("group is required for kafka_consumer_group %s", operation)
		}
		return "consumer_group.read " + group, nil
	case "reset_offset":
		group = strings.TrimSpace(group)
		if group == "" {
			return "", fmt.Errorf("group is required for kafka_consumer_group %s", operation)
		}
		return "consumer_group.offset.write " + group, nil
	case "delete":
		group = strings.TrimSpace(group)
		if group == "" {
			return "", fmt.Errorf("group is required for kafka_consumer_group %s", operation)
		}
		return "consumer_group.delete " + group, nil
	default:
		return "", fmt.Errorf("unsupported kafka_consumer_group operation: %s", operation)
	}
}

func kafkaACLCommand(operation string) (string, error) {
	switch operation {
	case "list":
		return "acl.read *", nil
	case "create", "delete":
		return "acl.write *", nil
	default:
		return "", fmt.Errorf("unsupported kafka_acl operation: %s", operation)
	}
}

func kafkaSchemaCommand(operation, subject string) (string, error) {
	switch operation {
	case "list_subjects":
		return "schema.read *", nil
	case "list_versions", "get", "describe", "check_compatibility":
		subject = strings.TrimSpace(subject)
		if subject == "" {
			return "", fmt.Errorf("subject is required for kafka_schema %s", operation)
		}
		return "schema.read " + subject, nil
	case "register":
		subject = strings.TrimSpace(subject)
		if subject == "" {
			return "", fmt.Errorf("subject is required for kafka_schema %s", operation)
		}
		return "schema.write " + subject, nil
	case "delete":
		subject = strings.TrimSpace(subject)
		if subject == "" {
			return "", fmt.Errorf("subject is required for kafka_schema %s", operation)
		}
		return "schema.delete " + subject, nil
	default:
		return "", fmt.Errorf("unsupported kafka_schema operation: %s", operation)
	}
}

func kafkaConnectCommand(operation, connector string) (string, error) {
	switch operation {
	case "list_clusters", "list_connectors":
		return "connect.read *", nil
	case "get_connector", "get", "describe":
		connector = strings.TrimSpace(connector)
		if connector == "" {
			return "", fmt.Errorf("connector is required for kafka_connect %s", operation)
		}
		return "connect.read " + connector, nil
	case "create", "update_config":
		connector = strings.TrimSpace(connector)
		if connector == "" {
			return "", fmt.Errorf("connector is required for kafka_connect %s", operation)
		}
		return "connect.write " + connector, nil
	case "pause", "resume", "restart":
		connector = strings.TrimSpace(connector)
		if connector == "" {
			return "", fmt.Errorf("connector is required for kafka_connect %s", operation)
		}
		return "connect.state.write " + connector, nil
	case "delete":
		connector = strings.TrimSpace(connector)
		if connector == "" {
			return "", fmt.Errorf("connector is required for kafka_connect %s", operation)
		}
		return "connect.delete " + connector, nil
	default:
		return "", fmt.Errorf("unsupported kafka_connect operation: %s", operation)
	}
}

func kafkaMessageCommand(operation, topic string) (string, error) {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return "", fmt.Errorf("topic is required for kafka_message %s", operation)
	}
	switch operation {
	case "browse", "inspect":
		return "message.read " + topic, nil
	case "produce":
		return "message.write " + topic, nil
	default:
		return "", fmt.Errorf("unsupported kafka_message operation: %s", operation)
	}
}
