import { describe, it, expect } from "vitest";
import { getAssetTypeOptions, matchSelectedTypes } from "@/lib/assetTypes/options";
import { asset_entity } from "../../wailsjs/go/models";

describe("getAssetTypeOptions", () => {
  it("returns built-in options when extensions registry is empty", () => {
    const opts = getAssetTypeOptions({});
    const values = opts.map((o) => o.value);
    expect(values).toEqual(["ssh", "database", "redis", "mongodb"]);
    expect(opts.every((o) => o.group === "builtin")).toBe(true);
  });

  it("aliases on database include mysql, postgresql, database", () => {
    const opts = getAssetTypeOptions({});
    const db = opts.find((o) => o.value === "database")!;
    expect(new Set(db.aliases)).toEqual(new Set(["database", "mysql", "postgresql"]));
  });

  it("merges extension assetTypes after built-ins", () => {
    const extensions = {
      k8sExt: {
        manifest: {
          name: "k8sExt",
          version: "1.0.0",
          icon: "Server",
          i18n: { displayName: "Kubernetes", description: "" },
          assetTypes: [{ type: "kubernetes", i18n: { name: "Kubernetes" } }],
        },
      },
    };
    const opts = getAssetTypeOptions(extensions as never);
    const ext = opts.find((o) => o.value === "kubernetes");
    expect(ext).toBeTruthy();
    expect(ext!.group).toBe("extension");
    expect(ext!.label).toBe("Kubernetes");
  });

  it("ignores extensions without assetTypes", () => {
    const extensions = {
      otherExt: {
        manifest: {
          name: "otherExt",
          version: "1.0.0",
          icon: "Box",
          i18n: { displayName: "Other", description: "" },
        },
      },
    };
    const opts = getAssetTypeOptions(extensions as never);
    expect(opts.filter((o) => o.group === "extension")).toEqual([]);
  });
});

describe("matchSelectedTypes", () => {
  const a = (id: number, type: string) => new asset_entity.Asset({ ID: id, Name: `n${id}`, Type: type });
  const assets = [a(1, "ssh"), a(2, "mysql"), a(3, "postgresql"), a(4, "redis"), a(5, "kubernetes")];
  const opts = getAssetTypeOptions({
    k8sExt: {
      manifest: {
        name: "k8sExt",
        version: "1",
        icon: "Server",
        i18n: { displayName: "Kubernetes", description: "" },
        assetTypes: [{ type: "kubernetes", i18n: { name: "Kubernetes" } }],
      },
    },
  } as never);

  it('returns all assets when selection is "all"', () => {
    expect(matchSelectedTypes(assets, "all", opts).map((x) => x.ID)).toEqual([1, 2, 3, 4, 5]);
  });

  it("matches database aliases (mysql, postgresql)", () => {
    expect(matchSelectedTypes(assets, ["database"], opts).map((x) => x.ID)).toEqual([2, 3]);
  });

  it("matches extension type", () => {
    expect(matchSelectedTypes(assets, ["kubernetes"], opts).map((x) => x.ID)).toEqual([5]);
  });

  it("treats empty selection as no filter (returns all)", () => {
    expect(matchSelectedTypes(assets, [], opts).map((x) => x.ID)).toEqual([1, 2, 3, 4, 5]);
  });

  it("matches case-insensitively", () => {
    const assetsMixed = [a(1, "SSH"), a(2, "MySQL")];
    expect(matchSelectedTypes(assetsMixed, ["ssh"], opts).map((x) => x.ID)).toEqual([1]);
    expect(matchSelectedTypes(assetsMixed, ["database"], opts).map((x) => x.ID)).toEqual([2]);
  });
});
