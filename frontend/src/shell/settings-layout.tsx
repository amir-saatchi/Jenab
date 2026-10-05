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
} from "@/components/ui/sidebar"
import { cn } from "@/lib/utils"

// Full-view settings (SPEC 5.12): a page list on the left, one scrolling
// page on the right. A page in the list scrolls to its section, whose id is
// sectionId(page), or calls onSelect when the page shown is another view.
// Wide pages, such as the developer tools, use the window's width.
export function SettingsLayout({
  label,
  items,
  active: initial,
  onSelect,
  onBack,
  wide,
  children,
}: {
  label?: string
  items: string[]
  active?: string
  onSelect?: (page: string) => void
  onBack: () => void
  wide?: boolean
  children: React.ReactNode
}) {
  const [active, setActive] = React.useState(initial ?? items[0])
  const show = (i: string) => {
    if (onSelect) return onSelect(i)
    setActive(i)
    document.getElementById(sectionId(i))?.scrollIntoView({ behavior: "smooth", block: "start" })
  }
  return (
    <div className="flex h-full min-h-0">
      <Sidebar collapsible="none" className="w-56 border-e max-[899px]:w-44">
        <SidebarHeader>
          <Button variant="ghost" size="sm" className="w-fit" onClick={onBack}>
            <ArrowLeftIcon data-icon="inline-start" className="rtl:rotate-180" />
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
                    <SidebarMenuButton isActive={i === active} onClick={() => show(i)}>
                      {i}
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>
      </Sidebar>
      <main className="flex min-w-0 flex-1">
        <ScrollArea className="h-full flex-1">
          <div className={cn("mx-auto flex flex-col gap-8 px-8 py-8", wide ? "max-w-6xl" : "max-w-2xl")}>{children}</div>
        </ScrollArea>
      </main>
    </div>
  )
}

export function sectionId(page: string) {
  return "settings-" + page.toLowerCase().replace(/[^a-z0-9]+/g, "-")
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
    <section id={sectionId(title)} className="flex scroll-mt-8 flex-col gap-3">
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
