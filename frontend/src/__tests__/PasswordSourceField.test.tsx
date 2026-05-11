import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import userEvent, { PointerEventsCheckLevel } from "@testing-library/user-event";
import { PasswordSourceField } from "../components/asset/PasswordSourceField";
import { credential_entity } from "../../wailsjs/go/models";

function makeCred(id: number, username: string): credential_entity.Credential {
  return { id, name: `cred-${id}`, username, type: "password" } as credential_entity.Credential;
}

// Radix Select renders SelectValue as a <span pointer-events:none> inside its trigger,
// so userEvent has to skip its pointer-events check before it can click the trigger.
function renderField(overrides: Partial<React.ComponentProps<typeof PasswordSourceField>> = {}) {
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never });
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
  return { ...render(<PasswordSourceField {...props} />), props, user };
}

describe("PasswordSourceField username 联动", () => {
  it("选中带 username 的密钥 → 触发 onUsernameChange", async () => {
    const { props, user } = renderField();

    await user.click(screen.getByText("asset.selectPasswordPlaceholder"));
    await user.click(screen.getByRole("option", { name: "cred-1 (alice)" }));

    expect(props.onCredentialIdChange).toHaveBeenCalledWith(1);
    expect(props.onUsernameChange).toHaveBeenCalledWith("alice");
  });

  it("选中 username 为空的密钥 → 不触发 onUsernameChange", async () => {
    const { props, user } = renderField();

    await user.click(screen.getByText("asset.selectPasswordPlaceholder"));
    await user.click(screen.getByRole("option", { name: "cred-2" }));

    expect(props.onCredentialIdChange).toHaveBeenCalledWith(2);
    expect(props.onUsernameChange).not.toHaveBeenCalled();
  });

  it("初次挂载（即使 credentialId 已有初值）→ 不触发 onUsernameChange", () => {
    const onUsernameChange = vi.fn();
    renderField({ credentialId: 1, onUsernameChange });
    expect(onUsernameChange).not.toHaveBeenCalled();
  });
});
