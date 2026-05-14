import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button, Input, Label, Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@opskat/ui";
import { RefreshCw } from "lucide-react";
import { ListSerialPorts } from "../../../wailsjs/go/app/App";

interface SerialPortInfo {
  name: string;
  displayName: string;
  productId?: string;
  vendorId?: string;
  serialNumber?: string;
}

export interface SerialConfigSectionProps {
  portPath: string;
  setPortPath: (v: string) => void;
  baudRate: number;
  setBaudRate: (v: number) => void;
  dataBits: number;
  setDataBits: (v: number) => void;
  stopBits: string;
  setStopBits: (v: string) => void;
  parity: string;
  setParity: (v: string) => void;
  flowControl: string;
  setFlowControl: (v: string) => void;
}

const CUSTOM_PORT = "__custom__";
const NO_PORTS_PLACEHOLDER = "__no_ports__";
const BAUD_RATES = [9600, 19200, 38400, 57600, 115200, 230400, 460800, 921600];
const DATA_BITS_OPTIONS = [5, 6, 7, 8];
const STOP_BITS_OPTIONS = ["1", "1.5", "2"];
const PARITY_OPTIONS = ["none", "odd", "even", "mark", "space"];
// "hardware" 走 serial_svc.enableHardwareFlowControl（直接 ioctl 设 CRTSCTS / DCB），
// 因为 go.bug.st/serial v1.6.4 自身不暴露这条配置，而 nativeOpen 会强制关闭它。
const FLOW_CONTROL_OPTIONS = ["none", "hardware"];

export function SerialConfigSection({
  portPath,
  setPortPath,
  baudRate,
  setBaudRate,
  dataBits,
  setDataBits,
  stopBits,
  setStopBits,
  parity,
  setParity,
  flowControl,
  setFlowControl,
}: SerialConfigSectionProps) {
  const { t } = useTranslation();
  const [ports, setPorts] = useState<SerialPortInfo[]>([]);
  const [loadingPorts, setLoadingPorts] = useState(false);
  const [customMode, setCustomMode] = useState(false);

  const fetchPorts = useCallback(async () => {
    setLoadingPorts(true);
    try {
      const list = await ListSerialPorts();
      setPorts(list || []);
    } catch {
      setPorts([]);
    } finally {
      setLoadingPorts(false);
    }
  }, []);

  useEffect(() => {
    fetchPorts();
  }, [fetchPorts]);

  // 已保存的端口在当前列表里没出现时（设备拔走、跨平台路径等），自动切到手动输入模式，
  // 让用户能看到原值。注意：这里只单向"开"不"关"——一旦进入手动模式就保留，
  // 用户主动从下拉里选了某个端口才会通过 handlePortSelect 切回非手动模式。
  // 这样刷新串口列表不会把正在编辑的内容覆盖掉。
  useEffect(() => {
    if (portPath && !ports.some((p) => p.name === portPath)) {
      setCustomMode(true);
    }
  }, [ports, portPath]);

  // Determine if current portPath matches a detected port
  const selectValue = customMode ? CUSTOM_PORT : portPath;

  const handlePortSelect = (value: string) => {
    if (value === CUSTOM_PORT) {
      setCustomMode(true);
      // Keep existing portPath so users can edit it; don't clear on mode switch
    } else {
      setCustomMode(false);
      setPortPath(value);
    }
  };

  return (
    <div className="grid gap-3 border rounded-lg p-4">
      <div className="grid gap-2">
        <div className="flex items-center justify-between">
          <Label>{t("asset.serialPortPath")}</Label>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="h-6 px-2 text-xs"
            onClick={fetchPorts}
            disabled={loadingPorts}
          >
            <RefreshCw className={`h-3 w-3 mr-1 ${loadingPorts ? "animate-spin" : ""}`} />
            {t("asset.serialRefreshPorts")}
          </Button>
        </div>
        <Select value={selectValue} onValueChange={handlePortSelect}>
          <SelectTrigger>
            <SelectValue placeholder={t("asset.serialPortPathPlaceholder")} />
          </SelectTrigger>
          <SelectContent>
            {ports.map((p) => (
              <SelectItem key={p.name} value={p.name}>
                {p.displayName}
                {p.serialNumber ? ` (${p.serialNumber})` : ""}
              </SelectItem>
            ))}
            {ports.length === 0 && !loadingPorts && (
              <SelectItem value={NO_PORTS_PLACEHOLDER} disabled>
                {t("asset.serialNoPortsDetected")}
              </SelectItem>
            )}
            <SelectItem value={CUSTOM_PORT}>{t("asset.serialManualInput")}</SelectItem>
          </SelectContent>
        </Select>
        {customMode && (
          <Input
            value={portPath}
            onChange={(e) => setPortPath(e.target.value)}
            placeholder={t("asset.serialPortPathPlaceholder")}
            className="font-mono"
          />
        )}
      </div>

      <div className="grid grid-cols-2 gap-3">
        <div className="grid gap-2">
          <Label>{t("asset.serialBaudRate")}</Label>
          <Select value={String(baudRate)} onValueChange={(v) => setBaudRate(Number(v))}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {BAUD_RATES.map((rate) => (
                <SelectItem key={rate} value={String(rate)}>
                  {rate}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="grid gap-2">
          <Label>{t("asset.serialDataBits")}</Label>
          <Select value={String(dataBits)} onValueChange={(v) => setDataBits(Number(v))}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {DATA_BITS_OPTIONS.map((bits) => (
                <SelectItem key={bits} value={String(bits)}>
                  {bits}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      <div className="grid grid-cols-3 gap-3">
        <div className="grid gap-2">
          <Label>{t("asset.serialStopBits")}</Label>
          <Select value={stopBits} onValueChange={setStopBits}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {STOP_BITS_OPTIONS.map((bits) => (
                <SelectItem key={bits} value={bits}>
                  {bits}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="grid gap-2">
          <Label>{t("asset.serialParity")}</Label>
          <Select value={parity} onValueChange={setParity}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {PARITY_OPTIONS.map((p) => (
                <SelectItem key={p} value={p}>
                  {p}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="grid gap-2">
          <Label>{t("asset.serialFlowControl")}</Label>
          <Select value={flowControl} onValueChange={setFlowControl}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {FLOW_CONTROL_OPTIONS.map((fc) => (
                <SelectItem key={fc} value={fc}>
                  {fc}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>
    </div>
  );
}
