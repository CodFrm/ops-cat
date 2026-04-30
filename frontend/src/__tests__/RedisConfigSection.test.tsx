import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { RedisConfigSection } from "../components/asset/RedisConfigSection";

const noop = vi.fn();

function renderSection(overrides: Partial<React.ComponentProps<typeof RedisConfigSection>> = {}) {
  return render(
    <RedisConfigSection
      host="127.0.0.1"
      setHost={noop}
      port={6379}
      setPort={noop}
      username=""
      setUsername={noop}
      tls={true}
      setTls={noop}
      tlsInsecure={false}
      setTlsInsecure={noop}
      tlsServerName=""
      setTlsServerName={noop}
      tlsCAFile=""
      setTlsCAFile={noop}
      tlsCertFile=""
      setTlsCertFile={noop}
      tlsKeyFile=""
      setTlsKeyFile={noop}
      database={1}
      setDatabase={noop}
      commandTimeoutSeconds={15}
      setCommandTimeoutSeconds={noop}
      scanPageSize={500}
      setScanPageSize={noop}
      keySeparator=":"
      setKeySeparator={noop}
      sshTunnelId={0}
      setSshTunnelId={noop}
      password=""
      setPassword={noop}
      encryptedPassword=""
      passwordSource="inline"
      setPasswordSource={noop}
      passwordCredentialId={0}
      setPasswordCredentialId={noop}
      managedPasswords={[]}
      {...overrides}
    />
  );
}

describe("RedisConfigSection", () => {
  it("renders redis browser and tls advanced settings", () => {
    renderSection();

    expect(screen.getByText("asset.redisDatabase")).toBeInTheDocument();
    expect(screen.getByText("asset.redisCommandTimeout")).toBeInTheDocument();
    expect(screen.getByText("asset.redisScanPageSize")).toBeInTheDocument();
    expect(screen.getByText("asset.redisKeySeparator")).toBeInTheDocument();
    expect(screen.getByText("asset.redisTlsInsecure")).toBeInTheDocument();
    expect(screen.getByText("asset.redisTlsServerName")).toBeInTheDocument();
    expect(screen.getByText("asset.redisTlsCAFile")).toBeInTheDocument();
    expect(screen.getByText("asset.redisTlsCertFile")).toBeInTheDocument();
    expect(screen.getByText("asset.redisTlsKeyFile")).toBeInTheDocument();
  });
});
