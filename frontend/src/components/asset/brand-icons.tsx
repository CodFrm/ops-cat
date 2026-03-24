import { Icon, type IconifyIcon } from "@iconify/react";

// Offline icon data imports (bundled at build time, no CDN)
// logos set: multi-colored official brand icons
import awsIcon from "@iconify-icons/logos/aws";
import azureIcon from "@iconify-icons/logos/microsoft-azure";
import gcpIcon from "@iconify-icons/logos/google-cloud";
import cloudflareIcon from "@iconify-icons/logos/cloudflare-icon";
import mysqlIcon from "@iconify-icons/logos/mysql-icon";
import postgresqlIcon from "@iconify-icons/logos/postgresql";
import redisIcon from "@iconify-icons/logos/redis";
import mongodbIcon from "@iconify-icons/logos/mongodb-icon";
import elasticsearchIcon from "@iconify-icons/logos/elasticsearch";
import mariadbIcon from "@iconify-icons/logos/mariadb-icon";
import rabbitmqIcon from "@iconify-icons/logos/rabbitmq-icon";
import etcdIcon from "@iconify-icons/logos/etcd";
import dockerIcon from "@iconify-icons/logos/docker-icon";
import kubernetesIcon from "@iconify-icons/logos/kubernetes";
import windowsIcon from "@iconify-icons/logos/microsoft-windows-icon";
import ubuntuIcon from "@iconify-icons/logos/ubuntu";
import centosIcon from "@iconify-icons/logos/centos-icon";
import debianIcon from "@iconify-icons/logos/debian";
import redhatIcon from "@iconify-icons/logos/redhat-icon";
import nginxIcon from "@iconify-icons/logos/nginx";
import grafanaIcon from "@iconify-icons/logos/grafana";
import prometheusIcon from "@iconify-icons/logos/prometheus";
// simple-icons set: monochrome brand icons (for providers not in logos)
import alicloudIcon from "@iconify-icons/simple-icons/alibabacloud";
import huaweiIcon from "@iconify-icons/simple-icons/huawei";
import clickhouseIcon from "@iconify-icons/simple-icons/clickhouse";
import kafkaIcon from "@iconify-icons/simple-icons/apachekafka";
import sqliteIcon from "@iconify-icons/simple-icons/sqlite";
import appleIcon from "@iconify-icons/simple-icons/apple";
import linuxIcon from "@iconify-icons/simple-icons/linux";
// tdesign set: Tencent's own design system (no Tencent Cloud logo in any icon set)
import tencentCloudIcon from "@iconify-icons/tdesign/cloud";

interface IconProps {
  className?: string;
  style?: React.CSSProperties;
}

function brandIcon(data: IconifyIcon | string) {
  const Component: React.FC<IconProps> = ({ className, style }) => (
    <Icon icon={data} className={className} style={style} />
  );
  return Component;
}

// ===== Cloud Providers =====
export const AwsIcon = brandIcon(awsIcon);
export const AzureIcon = brandIcon(azureIcon);
export const GcpIcon = brandIcon(gcpIcon);
export const AliCloudIcon = brandIcon(alicloudIcon);
export const TencentCloudIcon = brandIcon(tencentCloudIcon);
export const HuaweiCloudIcon = brandIcon(huaweiIcon);
export const CloudflareIcon = brandIcon(cloudflareIcon);

// ===== Databases & Middleware =====
export const MysqlIcon = brandIcon(mysqlIcon);
export const PostgresqlIcon = brandIcon(postgresqlIcon);
export const RedisIcon = brandIcon(redisIcon);
export const MongodbIcon = brandIcon(mongodbIcon);
export const ElasticsearchIcon = brandIcon(elasticsearchIcon);
export const KafkaIcon = brandIcon(kafkaIcon);
export const MariadbIcon = brandIcon(mariadbIcon);
export const SqliteIcon = brandIcon(sqliteIcon);
export const RabbitmqIcon = brandIcon(rabbitmqIcon);
export const EtcdIcon = brandIcon(etcdIcon);
export const ClickhouseIcon = brandIcon(clickhouseIcon);

// ===== System / OS =====
export const DockerIcon = brandIcon(dockerIcon);
export const KubernetesIcon = brandIcon(kubernetesIcon);
export const LinuxIcon = brandIcon(linuxIcon);
export const WindowsIcon = brandIcon(windowsIcon);
export const UbuntuIcon = brandIcon(ubuntuIcon);
export const CentosIcon = brandIcon(centosIcon);
export const DebianIcon = brandIcon(debianIcon);
export const RedhatIcon = brandIcon(redhatIcon);
export const MacosIcon = brandIcon(appleIcon);

// ===== DevOps & Monitoring =====
export const NginxIcon = brandIcon(nginxIcon);
export const GrafanaIcon = brandIcon(grafanaIcon);
export const PrometheusIcon = brandIcon(prometheusIcon);
