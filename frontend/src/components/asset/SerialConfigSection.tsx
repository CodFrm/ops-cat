import { useTranslation } from "react-i18next";
import { Input, Label, Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@opskat/ui";

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

const BAUD_RATES = [9600, 19200, 38400, 57600, 115200, 230400, 460800, 921600];
const DATA_BITS_OPTIONS = [5, 6, 7, 8];
const STOP_BITS_OPTIONS = ["1", "1.5", "2"];
const PARITY_OPTIONS = ["none", "odd", "even", "mark", "space"];
const FLOW_CONTROL_OPTIONS = ["none", "hardware", "software"];

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

  return (
    <div className="grid gap-3 border rounded-lg p-4">
      <div className="grid gap-2">
        <Label>{t("asset.serialPortPath")}</Label>
        <Input
          value={portPath}
          onChange={(e) => setPortPath(e.target.value)}
          placeholder={t("asset.serialPortPathPlaceholder")}
          className="font-mono"
        />
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
