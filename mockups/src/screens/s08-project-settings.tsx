import { FolderOpenIcon, TriangleAlertIcon } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Progress } from "@/components/ui/progress"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { FactRow, SettingsDivider, SettingsLayout, SettingsSection, SettingsTitle } from "@/jenab/settings"

const pages = ["General", "Workspace", "Storage", "Network", "Export and delete"]

const blocked = [".env*", "*.pem", "*.key", "id_*"]

const rules = [
  { prefix: "cache/pages/", keep: "30 days" },
  { prefix: "cache/runs/", keep: "14 days" },
  { prefix: "uploads/", keep: "forever" },
]

const hosts = [
  { host: "api.coingecko.com", since: "26 Sep" },
  { host: "feeds.example.com", since: "26 Sep" },
]

// 08 · Project settings: one scrolling page, opened from the project switcher (SPEC 5.12).
export function Screen08() {
  return (
    <SettingsLayout label="Project: Bitcoin" items={pages} active="General">
      <SettingsTitle title="Project settings" description="Bitcoin · created 26 Sep 2026" />

      <SettingsSection title="General">
        <FactRow label="Name">
          <Input defaultValue="Bitcoin" className="max-w-64" dir="auto" />
        </FactRow>
        <FactRow
          label="Folder"
          action={
            <Button variant="ghost" size="sm">
              <FolderOpenIcon data-icon="inline-start" />
              Show in Explorer
            </Button>
          }
        >
          <span className="block truncate font-mono text-xs">C:\Users\you\OneDrive\Jenab\bitcoin</span>
        </FactRow>
        <Alert className="[&>svg]:text-tone-amber">
          <TriangleAlertIcon />
          <AlertDescription className="flex flex-col items-start gap-2">
            This folder is inside OneDrive. Syncing can damage the database; use Export for backups.
            <Button variant="outline" size="xs">
              Move folder…
            </Button>
          </AlertDescription>
        </Alert>
      </SettingsSection>
      <SettingsDivider />

      <SettingsSection title="Workspace" description="The agent can only read the linked folder.">
        <FactRow
          label="Linked folder"
          action={
            <Button variant="outline" size="sm">
              Link folder…
            </Button>
          }
        >
          <span className="text-muted-foreground">none</span>
        </FactRow>
        <FactRow
          label="Blocked files"
          action={
            <Button variant="ghost" size="sm">
              Edit
            </Button>
          }
        >
          <div className="flex flex-wrap gap-1.5">
            {blocked.map((b) => (
              <Badge key={b} variant="outline" className="font-mono">
                {b}
              </Badge>
            ))}
          </div>
        </FactRow>
      </SettingsSection>
      <SettingsDivider />

      <SettingsSection title="Storage">
        <FactRow label="Cache">
          <div className="flex items-center gap-3">
            <Progress value={42} className="max-w-48" />
            <span className="text-xs text-muted-foreground tabular-nums">212 MB of 500 MB</span>
          </div>
        </FactRow>
        <FactRow
          label="Kept for undo"
          action={
            <Button variant="outline" size="sm">
              Free now
            </Button>
          }
        >
          <span className="tabular-nums">12 files, 48 MB</span>
        </FactRow>
        <div className="flex flex-col gap-2">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Prefix</TableHead>
                <TableHead>Keep</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rules.map((r) => (
                <TableRow key={r.prefix}>
                  <TableCell className="font-mono text-xs">{r.prefix}</TableCell>
                  <TableCell>{r.keep}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          <Button variant="ghost" size="sm" className="w-fit">
            Edit rules
          </Button>
        </div>
      </SettingsSection>
      <SettingsDivider />

      <SettingsSection title="Network" description="Hosts this project may call. Everything else is blocked.">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Allowed host</TableHead>
              <TableHead>Since</TableHead>
              <TableHead className="w-20" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {hosts.map((h) => (
              <TableRow key={h.host}>
                <TableCell className="font-mono text-xs">{h.host}</TableCell>
                <TableCell className="text-muted-foreground">{h.since}</TableCell>
                <TableCell className="py-0 text-end">
                  <Button variant="ghost" size="xs">
                    Remove
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        <FactRow
          label="Private network"
          action={
            <Button variant="ghost" size="sm">
              Add exception
            </Button>
          }
        >
          <span className="text-muted-foreground">blocked · no exceptions</span>
        </FactRow>
      </SettingsSection>
      <SettingsDivider />

      <SettingsSection title="Export and delete" description="Export makes a safe copy of the open project.">
        <div className="flex gap-2">
          <Button variant="outline" size="sm">
            Export project…
          </Button>
          <Button variant="destructive" size="sm">
            Delete project…
          </Button>
        </div>
      </SettingsSection>
    </SettingsLayout>
  )
}
