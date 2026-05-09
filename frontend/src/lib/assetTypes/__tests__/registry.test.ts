import { describe, it, expect } from "vitest";
import { getAssetType, isBuiltinType, getBuiltinTypes } from "../index";

describe("AssetType Registry", () => {
  it("registers all built-in types", () => {
    expect(getAssetType("ssh")).toBeDefined();
    expect(getAssetType("database")).toBeDefined();
    expect(getAssetType("redis")).toBeDefined();
    expect(getAssetType("mongodb")).toBeDefined();
    expect(getAssetType("kafka")).toBeDefined();
    expect(getAssetType("k8s")).toBeDefined();
  });

  it("returns undefined for unknown type", () => {
    expect(getAssetType("nonexistent")).toBeUndefined();
  });

  it("isBuiltinType", () => {
    expect(isBuiltinType("ssh")).toBe(true);
    expect(isBuiltinType("mongodb")).toBe(true);
    expect(isBuiltinType("kafka")).toBe(true);
    expect(isBuiltinType("k8s")).toBe(true);
    expect(isBuiltinType("unknown")).toBe(false);
  });

  it("getBuiltinTypes returns all built-in types", () => {
    expect(getBuiltinTypes().map((def) => def.type)).toEqual([
      "ssh",
      "database",
      "redis",
      "mongodb",
      "kafka",
      "k8s",
      "serial",
    ]);
  });

  it("each type has required fields", () => {
    for (const def of getBuiltinTypes()) {
      expect(def.type).toBeTruthy();
      expect(def.icon).toBeDefined();
      expect(typeof def.canConnect).toBe("boolean");
      expect(typeof def.canConnectInNewTab).toBe("boolean");
      expect(["terminal", "query"]).toContain(def.connectAction);
      expect(def.DetailInfoCard).toBeDefined();
    }
  });

  it("ssh and k8s are terminal, others are query", () => {
    expect(getAssetType("ssh")!.connectAction).toBe("terminal");
    expect(getAssetType("k8s")!.connectAction).toBe("terminal");
    expect(getAssetType("database")!.connectAction).toBe("query");
    expect(getAssetType("redis")!.connectAction).toBe("query");
    expect(getAssetType("mongodb")!.connectAction).toBe("query");
    expect(getAssetType("kafka")!.connectAction).toBe("query");
  });

  it("only ssh supports new tab", () => {
    expect(getAssetType("ssh")!.canConnectInNewTab).toBe(true);
    expect(getAssetType("database")!.canConnectInNewTab).toBe(false);
    expect(getAssetType("mongodb")!.canConnectInNewTab).toBe(false);
    expect(getAssetType("kafka")!.canConnectInNewTab).toBe(false);
    expect(getAssetType("k8s")!.canConnectInNewTab).toBe(false);
  });
});
