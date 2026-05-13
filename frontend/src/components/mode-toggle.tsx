import { Check, Moon, Sun } from "lucide-react";
import { useTranslation } from "react-i18next";
import {
  Button,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@opskat/ui";
import { useTheme, type Theme } from "@/components/theme-provider";

const THEME_OPTIONS: Theme[] = ["light", "dark", "system"];

export function ModeToggle() {
  const { theme, setTheme } = useTheme();
  const { t } = useTranslation();

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon">
          <Sun className="h-[1.2rem] w-[1.2rem] rotate-0 scale-100 transition-all dark:rotate-90 dark:scale-0" />
          <Moon className="absolute h-[1.2rem] w-[1.2rem] rotate-90 scale-0 transition-all dark:rotate-0 dark:scale-100" />
          <span className="sr-only">{t("theme.toggle")}</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-[8rem]">
        {THEME_OPTIONS.map((option) => (
          <DropdownMenuItem
            key={option}
            onClick={() => setTheme(option)}
            className="flex items-center justify-between gap-4"
          >
            <span>{t(`theme.${option}`)}</span>
            {theme === option && <Check className="size-4 text-muted-foreground" />}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
