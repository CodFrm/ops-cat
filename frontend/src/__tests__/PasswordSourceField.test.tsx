import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import userEvent, { PointerEventsCheckLevel } from "@testing-library/user-event";
import { PasswordSourceField } from "../components/asset/PasswordSourceField";
import { credential_entity } from "../../wailsjs/go/models";

function makeCred(id: number, username: string): credential_entity.Credential {
  return { id, name: `cred-${id}`, username, type: "password" } as credential_entity.Credential;
}

function renderField(overrides: Partial<React.ComponentProps<typeof PasswordSourceField>> = {}) {
  const props: React.ComponentProps<typeof PasswordSourceField> = {
    source: "managed",
    onSourceChange: vi.fn(),
    password: "",
    onPasswordChange: vi.fn(),
    credentialId: 0,
    onCredentialIdChange: vi.fn(),
    managedPasswords: [makeCred(1, "alice"), makeCred(2, ""), makeCred(3, "bob")],
    onUsernameChange: vi.fn(),
    ...overrides,
  };
  return { ...render(<PasswordSourceField {...props} />), props };
}

// Radix Select renders SelectValue as a <span pointer-events:none> inside a <button>.
// We disable pointer-events check so userEvent can click the trigger normally.
const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never });

describe("PasswordSourceField username 联动", () => {
  it("选中带 username 的密钥 → 触发 onUsernameChange", async () => {
    const { props } = renderField();

    // 打开 Select 列表（点击 SelectTrigger 的文本）
    await user.click(screen.getByText("asset.selectPasswordPlaceholder"));
    // 点击 "cred-1 (alice)"
    await user.click(screen.getByText("cred-1 (alice)"));

    expect(props.onCredentialIdChange).toHaveBeenCalledWith(1);
    expect(props.onUsernameChange).toHaveBeenCalledWith("alice");
  });

  it("选中 username 为空的密钥 → 不触发 onUsernameChange", async () => {
    const { props } = renderField();

    await user.click(screen.getByText("asset.selectPasswordPlaceholder"));
    await user.click(screen.getByText("cred-2"));

    expect(props.onCredentialIdChange).toHaveBeenCalledWith(2);
    expect(props.onUsernameChange).not.toHaveBeenCalled();
  });

  it("初次挂载（即使 credentialId 已有初值）→ 不触发 onUsernameChange", () => {
    const onUsernameChange = vi.fn();
    renderField({ credentialId: 1, onUsernameChange });
    expect(onUsernameChange).not.toHaveBeenCalled();
  });
});
