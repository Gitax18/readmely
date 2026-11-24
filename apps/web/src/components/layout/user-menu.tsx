"use client";

import { ChevronDown, CircleUser, LogOut } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import * as React from "react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { createAccountApi } from "@/lib/browser-api";

/** The signed-in email as a menu: the account page and signing out of this device. */
export function UserMenu({ email, csrf }: { email: string; csrf?: string }) {
  const router = useRouter();
  const [api] = React.useState(() => createAccountApi());

  async function signOut() {
    if (csrf) {
      const result = await api.logout(csrf);
      if (!result.ok && result.kind !== "unauthenticated") {
        router.push("/account");
        return;
      }
    }
    router.replace("/login");
    router.refresh();
  }

  const initial = email.slice(0, 1).toUpperCase();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        aria-label="Account menu"
        className="border-border hover:bg-surface-2 data-[state=open]:bg-surface-2 flex h-9 items-center gap-2 rounded-control border pr-2 pl-1 transition-colors"
      >
        <span
          aria-hidden
          className="bg-accent-soft text-accent grid size-7 place-items-center rounded-md text-xs font-bold"
        >
          {initial}
        </span>
        <span className="text-muted-foreground hidden max-w-36 truncate text-xs lg:block">
          {email}
        </span>
        <ChevronDown aria-hidden className="text-faint size-3.5" />
      </DropdownMenuTrigger>
      <DropdownMenuContent>
        <DropdownMenuLabel>{email}</DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem asChild>
          <Link href="/account">
            <CircleUser aria-hidden /> Account
          </Link>
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => void signOut()}>
          <LogOut aria-hidden /> Sign out
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
