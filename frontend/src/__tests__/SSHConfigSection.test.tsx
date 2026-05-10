import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import userEvent, { PointerEventsCheckLevel } from "@testing-library/user-event";
import { SSHConfigSection } from "../components/asset/SSHConfigSection";
import { credential_entity } from "../../wailsjs/go/models";

function makeCred(id: number, username: string): credential_entity.Credential {
  return { id, name: `cred-${id}`, username, type: "password" } as credential_entity.Credential;
}

// Radix Select renders SelectValue as a <span pointer-events:none> inside its trigger,
// so userEvent has to skip its pointer-events check before it can click the trigger.
function renderSSH(overrides: Partial<React.ComponentProps<typeof SSHConfigSection>> = {}) {
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never });
  const setUsername = vi.fn();
  const setPasswordCredentialId = vi.fn();
  const props: React.ComponentProps<typeof SSHConfigSection> = {
    host: "10.0.0.1",
    setHost: vi.fn(),
    port: 22,
    setPort: vi.fn(),
    username: "",
    setUsername,
    authType: "password",
    setAuthType: vi.fn(),
    connectionType: "direct",
    setConnectionType: vi.fn(),
    password: "",
    setPassword: vi.fn(),
    encryptedPassword: "",
    passwordSource: "managed",
    setPasswordSource: vi.fn(),
    passwordCredentialId: 0,
    setPasswordCredentialId,
    managedPasswords: [makeCred(1, "alice"), makeCred(2, "")],
    keySource: "managed",
    setKeySource: vi.fn(),
    credentialId: 0,
    setCredentialId: vi.fn(),
    managedKeys: [],
    localKeys: [],
    setLocalKeys: vi.fn(),
    selectedKeyPaths: [],
    setSelectedKeyPaths: vi.fn(),
    privateKeyPassphrase: "",
    setPrivateKeyPassphrase: vi.fn(),
    scanningKeys: false,
    sshTunnelId: 0,
    setSshTunnelId: vi.fn(),
    proxyType: "",
    setProxyType: vi.fn(),
    proxyHost: "",
    setProxyHost: vi.fn(),
    proxyPort: 0,
    setProxyPort: vi.fn(),
    proxyUsername: "",
    setProxyUsername: vi.fn(),
    proxyPassword: "",
    setProxyPassword: vi.fn(),
    encryptedProxyPassword: "",
    ...overrides,
  };
  return { ...render(<SSHConfigSection {...props} />), setUsername, setPasswordCredentialId, user };
}

describe("SSHConfigSection 自动填用户名", () => {
  it("选中带 username 的密钥时调 setUsername", async () => {
    const { setUsername, setPasswordCredentialId, user } = renderSSH();

    await user.click(screen.getByText("asset.selectPasswordPlaceholder"));
    await user.click(screen.getByRole("option", { name: "cred-1 (alice)" }));

    expect(setPasswordCredentialId).toHaveBeenCalledWith(1);
    expect(setUsername).toHaveBeenCalledWith("alice");
  });

  it("选中 username 为空的密钥时不调 setUsername", async () => {
    const { setUsername, user } = renderSSH({
      username: "preexisting",
      managedPasswords: [makeCred(2, "")],
    });

    await user.click(screen.getByText("asset.selectPasswordPlaceholder"));
    await user.click(screen.getByRole("option", { name: "cred-2" }));

    expect(setUsername).not.toHaveBeenCalled();
  });
});
