import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, within } from "@testing-library/react";
import { asset_entity } from "../../../../../wailsjs/go/models";
import { SSHDetailInfoCard } from "../SSHDetailInfoCard";
import { DatabaseDetailInfoCard } from "../DatabaseDetailInfoCard";
import { RedisDetailInfoCard } from "../RedisDetailInfoCard";
import { MongoDBDetailInfoCard } from "../MongoDBDetailInfoCard";
import { K8sDetailInfoCard } from "../K8sDetailInfoCard";
import { SerialDetailInfoCard } from "../SerialDetailInfoCard";

afterEach(() => {
  cleanup();
});

function makeAsset(type: string, config: Record<string, unknown>): asset_entity.Asset {
  return new asset_entity.Asset({
    ID: 1,
    Name: "test-asset",
    Type: type,
    Config: JSON.stringify(config),
    CmdPolicy: "",
    GroupID: 0,
    Icon: "",
    Tags: "",
    Description: "",
    SortOrder: 0,
    sshTunnelId: 0,
    Status: 1,
    Createtime: 0,
    Updatetime: 0,
  });
}

function makeAssetWithTunnel(type: string, config: Record<string, unknown>, sshTunnelId: number): asset_entity.Asset {
  const asset = makeAsset(type, config);
  asset.sshTunnelId = sshTunnelId;
  return asset;
}

const noopTunnel = vi.fn(() => null);

describe("SSHDetailInfoCard", () => {
  it("renders SSH connection fields", () => {
    const asset = makeAsset("ssh", {
      host: "10.0.0.1",
      port: 22,
      username: "root",
      auth_type: "password",
      password: "secret",
    });
    const { getByText } = render(<SSHDetailInfoCard asset={asset} sshTunnelName={noopTunnel} />);
    expect(getByText("10.0.0.1")).toBeInTheDocument();
    expect(getByText("22")).toBeInTheDocument();
    expect(getByText("root")).toBeInTheDocument();
  });

  it("renders jump host when present", () => {
    const tunnelFn = vi.fn((id?: number) => (id === 5 ? "jump-server" : null));
    const asset = makeAsset("ssh", {
      host: "10.0.0.1",
      port: 22,
      username: "root",
      auth_type: "key",
      jump_host_id: 5,
    });
    const { getByText } = render(<SSHDetailInfoCard asset={asset} sshTunnelName={tunnelFn} />);
    expect(getByText("jump-server")).toBeInTheDocument();
  });

  it("renders proxy section when present", () => {
    const asset = makeAsset("ssh", {
      host: "10.0.0.1",
      port: 22,
      username: "root",
      auth_type: "password",
      proxy: { type: "socks5", host: "proxy.example.com", port: 1080, username: "proxyuser" },
    });
    const { getByText } = render(<SSHDetailInfoCard asset={asset} sshTunnelName={noopTunnel} />);
    expect(getByText("SOCKS5")).toBeInTheDocument();
    expect(getByText("proxy.example.com:1080")).toBeInTheDocument();
    expect(getByText("proxyuser")).toBeInTheDocument();
  });

  it("handles empty config without crashing", () => {
    const asset = makeAsset("ssh", {});
    const { container } = render(<SSHDetailInfoCard asset={asset} sshTunnelName={noopTunnel} />);
    expect(container).toBeDefined();
  });
});

describe("DatabaseDetailInfoCard", () => {
  it("renders database connection fields", () => {
    const asset = makeAsset("database", {
      driver: "postgresql",
      host: "db.example.com",
      port: 5432,
      username: "admin",
      password: "secret",
      database: "mydb",
    });
    const { getByText } = render(<DatabaseDetailInfoCard asset={asset} sshTunnelName={noopTunnel} />);
    expect(getByText("PostgreSQL")).toBeInTheDocument();
    expect(getByText("db.example.com:5432")).toBeInTheDocument();
    expect(getByText("admin")).toBeInTheDocument();
    expect(getByText("mydb")).toBeInTheDocument();
  });

  it("masks password field", () => {
    const asset = makeAsset("database", {
      driver: "mysql",
      host: "db.example.com",
      port: 3306,
      username: "admin",
      password: "supersecret",
    });
    const { container, queryByText } = render(<DatabaseDetailInfoCard asset={asset} sshTunnelName={noopTunnel} />);
    const view = within(container);
    expect(view.getByText("\u25CF\u25CF\u25CF\u25CF\u25CF\u25CF")).toBeInTheDocument();
    expect(queryByText("supersecret")).not.toBeInTheDocument();
  });

  it("shows SSH tunnel when present", () => {
    const tunnelFn = vi.fn((id?: number) => (id === 3 ? "bastion" : null));
    const asset = makeAsset("database", {
      driver: "mysql",
      host: "db.local",
      port: 3306,
      username: "root",
      ssh_asset_id: 3,
    });
    const { getByText } = render(<DatabaseDetailInfoCard asset={asset} sshTunnelName={tunnelFn} />);
    expect(getByText("bastion")).toBeInTheDocument();
  });

  it("handles empty config without crashing", () => {
    const asset = makeAsset("database", {});
    const { container } = render(<DatabaseDetailInfoCard asset={asset} sshTunnelName={noopTunnel} />);
    expect(container).toBeDefined();
  });
});

describe("RedisDetailInfoCard", () => {
  it("renders redis connection fields", () => {
    const asset = makeAsset("redis", {
      host: "redis.example.com",
      port: 6379,
      username: "default",
      password: "redispw",
      database: 2,
    });
    const { getByText } = render(<RedisDetailInfoCard asset={asset} sshTunnelName={noopTunnel} />);
    expect(getByText("redis.example.com:6379")).toBeInTheDocument();
    expect(getByText("default")).toBeInTheDocument();
    expect(getByText("2")).toBeInTheDocument();
  });

  it("masks password field", () => {
    const asset = makeAsset("redis", {
      host: "redis.local",
      port: 6379,
      password: "topsecret",
    });
    const { container, queryByText } = render(<RedisDetailInfoCard asset={asset} sshTunnelName={noopTunnel} />);
    const view = within(container);
    expect(view.getByText("\u25CF\u25CF\u25CF\u25CF\u25CF\u25CF")).toBeInTheDocument();
    expect(queryByText("topsecret")).not.toBeInTheDocument();
  });

  it("handles empty config without crashing", () => {
    const asset = makeAsset("redis", {});
    const { container } = render(<RedisDetailInfoCard asset={asset} sshTunnelName={noopTunnel} />);
    expect(container).toBeDefined();
  });
});

describe("MongoDBDetailInfoCard", () => {
  it("renders MongoDB fields with host:port", () => {
    const asset = makeAsset("mongodb", {
      host: "mongo.example.com",
      port: 27017,
      username: "mongouser",
      password: "mongopw",
      database: "testdb",
      auth_source: "admin",
    });
    const { getByText } = render(<MongoDBDetailInfoCard asset={asset} sshTunnelName={noopTunnel} />);
    expect(getByText("mongo.example.com:27017")).toBeInTheDocument();
    expect(getByText("mongouser")).toBeInTheDocument();
    expect(getByText("testdb")).toBeInTheDocument();
    expect(getByText("admin")).toBeInTheDocument();
  });

  it("shows URI when connection_uri is present", () => {
    const asset = makeAsset("mongodb", {
      connection_uri: "mongodb+srv://user:pass@cluster.example.com/mydb",
    });
    const { getByText } = render(<MongoDBDetailInfoCard asset={asset} sshTunnelName={noopTunnel} />);
    expect(getByText("mongodb+srv://user:pass@cluster.example.com/mydb")).toBeInTheDocument();
  });

  it("masks password field", () => {
    const asset = makeAsset("mongodb", {
      host: "mongo.local",
      port: 27017,
      password: "secretpw",
    });
    const { container, queryByText } = render(<MongoDBDetailInfoCard asset={asset} sshTunnelName={noopTunnel} />);
    const view = within(container);
    expect(view.getByText("\u25CF\u25CF\u25CF\u25CF\u25CF\u25CF")).toBeInTheDocument();
    expect(queryByText("secretpw")).not.toBeInTheDocument();
  });

  it("handles empty config without crashing", () => {
    const asset = makeAsset("mongodb", {});
    const { container } = render(<MongoDBDetailInfoCard asset={asset} sshTunnelName={noopTunnel} />);
    expect(container).toBeDefined();
  });
});

describe("SerialDetailInfoCard", () => {
  it("renders serial configuration fields", () => {
    const asset = makeAsset("serial", {
      port_path: "COM3",
      baud_rate: 115200,
      data_bits: 8,
      stop_bits: "1",
      parity: "none",
      flow_control: "hardware",
    });
    const { getByText } = render(<SerialDetailInfoCard asset={asset} />);
    expect(getByText("COM3")).toBeInTheDocument();
    expect(getByText("115200")).toBeInTheDocument();
    expect(getByText("8")).toBeInTheDocument();
    expect(getByText("1")).toBeInTheDocument();
    expect(getByText("none")).toBeInTheDocument();
    expect(getByText("hardware")).toBeInTheDocument();
  });

  it("hides flow control when set to none", () => {
    const asset = makeAsset("serial", {
      port_path: "COM4",
      baud_rate: 9600,
      flow_control: "none",
    });
    const { queryByText } = render(<SerialDetailInfoCard asset={asset} />);
    expect(queryByText("none")).not.toBeInTheDocument();
  });

  it("handles empty config without crashing", () => {
    const asset = makeAsset("serial", {});
    const { container } = render(<SerialDetailInfoCard asset={asset} />);
    expect(container).toBeDefined();
  });

  it("handles invalid JSON safely", () => {
    const asset = makeAsset("serial", {});
    asset.Config = "{";
    const { container, queryByText } = render(<SerialDetailInfoCard asset={asset} />);
    expect(container).toBeDefined();
    expect(queryByText("串口")).not.toBeInTheDocument();
  });
});

describe("K8sDetailInfoCard", () => {
  it("shows SSH tunnel from asset field", () => {
    const tunnelFn = vi.fn((id?: number) => (id === 7 ? "k8s-bastion" : null));
    const asset = makeAssetWithTunnel("k8s", { kubeconfig: "apiVersion: v1" }, 7);
    const { getByText } = render(<K8sDetailInfoCard asset={asset} sshTunnelName={tunnelFn} />);
    expect(getByText("k8s-bastion")).toBeInTheDocument();
  });
});
