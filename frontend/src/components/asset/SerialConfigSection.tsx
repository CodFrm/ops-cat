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
const BAUD_RATES = [9600, 19200, 38400, 57600, 115200, 230400, 460800, 921600];
const DATA_BITS_OPTIONS = [5, 6, 7, 8];
const STOP_BITS_OPTIONS = ["1", "1.5", "2"];
const PARITY_OPTIONS = ["none", "odd", "even", "mark", "space"];
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

  // Auto-enable custom mode if the current portPath is not in the detected list
  useEffect(() => {
    if (ports.length > 0 && portPath && !ports.some((p) => p.name === portPath)) {
      setCustomMode(true);
    }
  }, [ports, portPath]);

  // Determine if current portPath matches a detected port
  const selectValue = customMode ? CUSTOM_PORT : portPath;

  const handlePortSelect = (value: string) => {
    if (value === CUSTOM_PORT) {
      setCustomMode(true);
      setPortPath("");
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
              <SelectItem value={CUSTOM_PORT} disabled>
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
