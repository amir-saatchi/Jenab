import * as React from "react"
import { ArrowLeftIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Separator } from "@/components/ui/separator"
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
} from "@/components/ui/sidebar"
import { cn } from "@/lib/utils"

// Full-view settings (07, 08): a page list on the left, one scrolling page on the right.
export function SettingsLayout({
  label,
  items,
  active,
  aside,
  wide,
  children,
}: {
  wide?: boolean
  label?: string
  items: string[]
  active: string
  aside?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <SidebarProvider className="h-svh overflow-hidden">
      <Sidebar collapsible="none" className="w-56 border-e">
        <SidebarHeader>
          <Button variant="ghost" size="sm" className="w-fit">
            <ArrowLeftIcon data-icon="inline-start" />
            Back to chats
          </Button>
        </SidebarHeader>
        <SidebarContent>
          <SidebarGroup>
            {label && <SidebarGroupLabel>{label}</SidebarGroupLabel>}
            <SidebarGroupContent>
              <SidebarMenu>
                {items.map((i) => (
                  <SidebarMenuItem key={i}>
                    <SidebarMenuButton isActive={i === active}>{i}</SidebarMenuButton>
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>
      </Sidebar>
      <main className="flex min-w-0 flex-1">
        <ScrollArea className="h-full flex-1">
          <div className={cn("mx-auto flex flex-col gap-8 px-8 py-8", wide ? "max-w-6xl" : "max-w-2xl")}>
            {children}
          </div>
        </ScrollArea>
        {aside}
      </main>
    </SidebarProvider>
  )
}

export function SettingsTitle({ title, description }: { title: string; description?: string }) {
  return (
    <div className="flex flex-col gap-1">
      <h1 className="text-xl font-semibold tracking-tight">{title}</h1>
      {description && <p className="text-sm text-muted-foreground">{description}</p>}
    </div>
  )
}

export function SettingsSection({
  title,
  description,
  children,
}: {
  title: string
  description?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <section className="flex flex-col gap-3">
      <div className="flex flex-col gap-0.5">
        <h2 className="text-sm font-medium">{title}</h2>
        {description && <p className="text-sm text-muted-foreground">{description}</p>}
      </div>
      {children}
    </section>
  )
}

export function SettingsDivider() {
  return <Separator />
}

// A row of label and value, used for read-only facts (project settings).
export function FactRow({
  label,
  children,
  action,
}: {
  label: string
  children: React.ReactNode
  action?: React.ReactNode
}) {
  return (
    <div className="grid grid-cols-[9rem_1fr_auto] items-center gap-4 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <div className="min-w-0">{children}</div>
      <div className="flex gap-2">{action}</div>
    </div>
  )
}
