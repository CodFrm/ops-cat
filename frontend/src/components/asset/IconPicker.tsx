import { useState, useMemo } from "react";
import { useTranslation } from "react-i18next";
import {
  Server,
  Database,
  Cloud,
  Monitor,
  Laptop,
  Router,
  HardDrive,
  Globe,
  Shield,
  Container,
  Cpu,
  Network,
  Folder,
  FolderOpen,
  FolderHeart,
  Archive,
  Box,
  Layers,
  Search,
  ChevronDown,
  Ban,
} from "lucide-react";
import {
  cn,
  Popover,
  PopoverContent,
  PopoverTrigger,
  Input,
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
  Button,
} from "@opskat/ui";
import {
  AwsIcon,
  AzureIcon,
  GcpIcon,
  AliCloudIcon,
  TencentCloudIcon,
  HuaweiCloudIcon,
  CloudflareIcon,
  MysqlIcon,
  PostgresqlIcon,
  RedisIcon,
  MongodbIcon,
  ElasticsearchIcon,
  KafkaIcon,
  MariadbIcon,
  SqliteIcon,
  RabbitmqIcon,
  EtcdIcon,
  ClickhouseIcon,
  DockerIcon,
  KubernetesIcon,
  LinuxIcon,
  WindowsIcon,
  UbuntuIcon,
  CentosIcon,
  DebianIcon,
  RedhatIcon,
  MacosIcon,
  NginxIcon,
  GrafanaIcon,
  PrometheusIcon,
} from "./brand-icons";

type IconComponent = React.ComponentType<{ className?: string; style?: React.CSSProperties }>;

interface IconCategory {
  key: string;
  icons: Record<string, IconComponent>;
}

// Display names for tooltips (proper nouns, language-independent)
const ICON_DISPLAY_NAMES: Record<string, string> = {
  server: "Server",
  database: "Database",
  cloud: "Cloud",
  monitor: "Monitor",
  laptop: "Laptop",
  router: "Router",
  "hard-drive": "Hard Drive",
  globe: "Globe",
  shield: "Shield",
  container: "Container",
  cpu: "CPU",
  network: "Network",
  aws: "AWS",
  azure: "Azure",
  gcp: "Google Cloud",
  alicloud: "Alibaba Cloud",
  tencentcloud: "Tencent Cloud",
  huaweicloud: "Huawei Cloud",
  cloudflare: "Cloudflare",
  mysql: "MySQL",
  postgresql: "PostgreSQL",
  redis: "Redis",
  mongodb: "MongoDB",
  elasticsearch: "Elasticsearch",
  kafka: "Kafka",
  mariadb: "MariaDB",
  sqlite: "SQLite",
  rabbitmq: "RabbitMQ",
  etcd: "etcd",
  clickhouse: "ClickHouse",
  docker: "Docker",
  kubernetes: "Kubernetes",
  linux: "Linux",
  windows: "Windows",
  ubuntu: "Ubuntu",
  centos: "CentOS",
  debian: "Debian",
  redhat: "Red Hat",
  macos: "macOS",
  nginx: "Nginx",
  grafana: "Grafana",
  prometheus: "Prometheus",
  folder: "Folder",
  "folder-open": "Folder Open",
  "folder-heart": "Folder Heart",
  archive: "Archive",
  box: "Box",
  layers: "Layers",
};

const CATEGORIES: IconCategory[] = [
  {
    key: "infrastructure",
    icons: {
      server: Server,
      database: Database,
      cloud: Cloud,
      monitor: Monitor,
      laptop: Laptop,
      router: Router,
      "hard-drive": HardDrive,
      globe: Globe,
      shield: Shield,
      container: Container,
      cpu: Cpu,
      network: Network,
    },
  },
  {
    key: "cloud",
    icons: {
      aws: AwsIcon,
      azure: AzureIcon,
      gcp: GcpIcon,
      alicloud: AliCloudIcon,
      tencentcloud: TencentCloudIcon,
      huaweicloud: HuaweiCloudIcon,
      cloudflare: CloudflareIcon,
    },
  },
  {
    key: "database",
    icons: {
      mysql: MysqlIcon,
      postgresql: PostgresqlIcon,
      redis: RedisIcon,
      mongodb: MongodbIcon,
      elasticsearch: ElasticsearchIcon,
      kafka: KafkaIcon,
      mariadb: MariadbIcon,
      sqlite: SqliteIcon,
      rabbitmq: RabbitmqIcon,
      etcd: EtcdIcon,
      clickhouse: ClickhouseIcon,
    },
  },
  {
    key: "system",
    icons: {
      docker: DockerIcon,
      kubernetes: KubernetesIcon,
      linux: LinuxIcon,
      windows: WindowsIcon,
      ubuntu: UbuntuIcon,
      centos: CentosIcon,
      debian: DebianIcon,
      redhat: RedhatIcon,
      macos: MacosIcon,
    },
  },
  {
    key: "devops",
    icons: {
      nginx: NginxIcon,
      grafana: GrafanaIcon,
      prometheus: PrometheusIcon,
    },
  },
  {
    key: "folder",
    icons: {
      folder: Folder,
      "folder-open": FolderOpen,
      "folder-heart": FolderHeart,
      archive: Archive,
      box: Box,
      layers: Layers,
    },
  },
];

// Brand colors for monochrome icons (simple-icons / tdesign).
// logos set icons already have colors baked in, so only these need explicit colors.
const ICON_COLORS: Record<string, string> = {
  alicloud: "#FF6A00",
  tencentcloud: "#00A4FF",
  huaweicloud: "#CF0A2C",
  clickhouse: "#FFCC01",
  sqlite: "#0F80CC",
  linux: "#FCC624",
  macos: "#A2AAAD",
};

// Preset colors for custom color selection
const PRESET_COLORS = [
  "#EF4444", // red
  "#F97316", // orange
  "#F59E0B", // amber
  "#84CC16", // lime
  "#22C55E", // green
  "#14B8A6", // teal
  "#06B6D4", // cyan
  "#3B82F6", // blue
  "#6366F1", // indigo
  "#8B5CF6", // violet
  "#A855F7", // purple
  "#EC4899", // pink
  "#78716C", // stone
  "#64748B", // slate
];

// Parse icon value: "iconName" or "iconName#hexcolor"
function parseIconValue(value: string): { name: string; color?: string } {
  const idx = value.indexOf("#");
  if (idx === -1) return { name: value };
  return { name: value.substring(0, idx), color: value.substring(idx) };
}

// Build icon value from name and optional color
function buildIconValue(name: string, color?: string): string {
  return color ? `${name}${color}` : name;
}

// Module-level icon registry for getIconComponent
const ALL_ICONS: Record<string, IconComponent> = {};
for (const cat of CATEGORIES) {
  Object.assign(ALL_ICONS, cat.icons);
}

interface IconPickerProps {
  value: string;
  onChange: (icon: string) => void;
  type?: "asset" | "group";
  /** compact: 仅显示图标的方形触发按钮，无文字、无下拉箭头，便于和输入框同排。 */
  compact?: boolean;
}

export function IconPicker({ value, onChange, type = "asset", compact = false }: IconPickerProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");

  const { name: iconName, color: customColor } = parseIconValue(value);
  const resolvedColor = customColor || ICON_COLORS[iconName];

  // For groups, put folder category first; for assets, put it last
  const categories = useMemo(() => {
    if (type === "group") {
      const folder = CATEGORIES.find((c) => c.key === "folder")!;
      const rest = CATEGORIES.filter((c) => c.key !== "folder");
      return [folder, ...rest];
    }
    return CATEGORIES;
  }, [type]);

  const SelectedIcon = ALL_ICONS[iconName] || Server;
  const displayName = ICON_DISPLAY_NAMES[iconName] || iconName;

  // Filter categories by search query
  const filteredCategories = useMemo(() => {
    if (!search) return categories;
    const q = search.toLowerCase();
    return categories
      .map((cat) => ({
        ...cat,
        icons: Object.fromEntries(
          Object.entries(cat.icons).filter(([name]) => {
            const display = ICON_DISPLAY_NAMES[name] || name;
            return name.includes(q) || display.toLowerCase().includes(q);
          })
        ),
      }))
      .filter((cat) => Object.keys(cat.icons).length > 0);
  }, [categories, search]);

  const handleSelect = (name: string) => {
    onChange(buildIconValue(name, customColor));
  };

  const handleColorSelect = (color: string | null) => {
    onChange(buildIconValue(iconName, color || undefined));
  };

  return (
    <Popover
      open={open}
      onOpenChange={(v) => {
        setOpen(v);
        if (!v) setSearch("");
      }}
    >
      <PopoverTrigger asChild>
        {compact ? (
          <Button
            variant="outline"
            role="combobox"
            aria-label={displayName}
            className="h-9 w-9 shrink-0 p-0 flex items-center justify-center"
          >
            <SelectedIcon className="h-4 w-4" style={resolvedColor ? { color: resolvedColor } : undefined} />
          </Button>
        ) : (
          <Button variant="outline" role="combobox" className="w-full justify-between font-normal h-9">
            <div className="flex items-center gap-2">
              <SelectedIcon
                className="h-4 w-4 shrink-0"
                style={resolvedColor ? { color: resolvedColor } : undefined}
              />
              <span className="truncate">{displayName}</span>
            </div>
            <ChevronDown className="ml-auto h-4 w-4 shrink-0 opacity-50" />
          </Button>
        )}
      </PopoverTrigger>
      <PopoverContent className="w-[320px] p-0" align="start">
        <TooltipProvider delayDuration={400}>
          <div className="p-2 pb-1">
            <div className="relative">
              <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
              <Input
                placeholder={t("asset.iconSearch")}
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="pl-8 h-8 text-sm"
              />
            </div>
          </div>
          <div className="max-h-[280px] overflow-y-auto" onWheel={(e) => e.stopPropagation()}>
            <div className="p-2 pt-1 space-y-2">
              {filteredCategories.length === 0 && (
                <div className="text-center text-sm text-muted-foreground py-6">{t("asset.iconNoResults")}</div>
              )}
              {filteredCategories.map((cat) => (
                <div key={cat.key}>
                  <div className="text-[11px] font-medium text-muted-foreground px-0.5 mb-1">
                    {t(`asset.iconCategory.${cat.key}`)}
                  </div>
                  <div className="flex flex-wrap gap-1">
                    {Object.entries(cat.icons).map(([name, Icon]) => {
                      const brandColor = ICON_COLORS[name];
                      const isSelected = iconName === name;
                      return (
                        <Tooltip key={name}>
                          <TooltipTrigger asChild>
                            <button
                              type="button"
                              className={cn(
                                "flex h-8 w-8 items-center justify-center rounded-md transition-colors",
                                isSelected
                                  ? "bg-primary text-primary-foreground"
                                  : "hover:bg-muted text-muted-foreground hover:text-foreground"
                              )}
                              onClick={() => handleSelect(name)}
                            >
                              <Icon
                                className="h-4 w-4"
                                style={brandColor && !isSelected ? { color: brandColor } : undefined}
                              />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent side="bottom" className="text-xs px-2 py-1">
                            {ICON_DISPLAY_NAMES[name] || name}
                          </TooltipContent>
                        </Tooltip>
                      );
                    })}
                  </div>
                </div>
              ))}
            </div>
          </div>
          {/* Color picker */}
          <div className="border-t p-2 space-y-1.5">
            <div className="text-[11px] font-medium text-muted-foreground px-0.5">{t("asset.iconColor")}</div>
            <div className="flex flex-wrap gap-1.5 items-center">
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    type="button"
                    className={cn(
                      "flex h-6 w-6 items-center justify-center rounded-full border transition-colors",
                      !customColor
                        ? "ring-2 ring-primary ring-offset-1 ring-offset-background"
                        : "hover:ring-2 hover:ring-muted-foreground/30 hover:ring-offset-1 hover:ring-offset-background"
                    )}
                    onClick={() => handleColorSelect(null)}
                  >
                    <Ban className="h-3.5 w-3.5 text-muted-foreground" />
                  </button>
                </TooltipTrigger>
                <TooltipContent side="bottom" className="text-xs px-2 py-1">
                  {t("asset.iconColorDefault")}
                </TooltipContent>
              </Tooltip>
              {PRESET_COLORS.map((color) => (
                <button
                  key={color}
                  type="button"
                  className={cn(
                    "h-6 w-6 rounded-full transition-all",
                    customColor === color
                      ? "ring-2 ring-primary ring-offset-1 ring-offset-background"
                      : "hover:ring-2 hover:ring-muted-foreground/30 hover:ring-offset-1 hover:ring-offset-background"
                  )}
                  style={{ backgroundColor: color }}
                  onClick={() => handleColorSelect(color)}
                />
              ))}
            </div>
          </div>
        </TooltipProvider>
      </PopoverContent>
    </Popover>
  );
}

// Get icon component by name (supports "name#color" format)
// eslint-disable-next-line react-refresh/only-export-components
export function getIconComponent(value: string): IconComponent {
  const { name } = parseIconValue(value);
  return ALL_ICONS[name] || Server;
}

// Get color for an icon (supports "name#color" format, falls back to brand color)
// eslint-disable-next-line react-refresh/only-export-components
export function getIconColor(value: string): string | undefined {
  const { name, color } = parseIconValue(value);
  return color || ICON_COLORS[name];
}
