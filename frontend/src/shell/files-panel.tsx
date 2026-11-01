import * as React from "react"
import {
  DatabaseIcon,
  ExternalLinkIcon,
  FileIcon,
  FileImageIcon,
  FileTextIcon,
  FolderIcon,
  LinkIcon,
  MoreHorizontalIcon,
  RefreshCwIcon,
  Trash2Icon,
} from "lucide-react"

import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty"
import {
  SidebarGroup,
  SidebarGroupAction,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuAction,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSkeleton,
} from "@/components/ui/sidebar"
import { BucketService, type FileObject } from "@/lib/api"
import { toUIError } from "@/lib/errors"
import { CACHE, crumbs, fileKind, nameOf, shownFolders } from "@/lib/files"
import { chatKey, useDock } from "@/state/dock"
import { useFiles } from "@/state/files"
import { current, useNav } from "@/state/nav"
import { copyLink, deleteFile, openFile, openWithSystem } from "@/shell/open-file"

type List = (project: string, prefix: string, after: string) => Promise<{ folders: string[] | null; objects: FileObject[] | null; next: string }>

interface Folder {
  folders: string[]
  objects: FileObject[]
  next: string // "" when all is loaded
}

// listFolder loads a folder's first page, or with after the next one, and
// adds it to what is shown. The top folder leaves cache/ out (SPEC 5.12).
export async function listFolder(list: List, project: string, prefix: string, shown: Folder | null, after = ""): Promise<Folder> {
  const pg = await list(project, prefix, after)
  return {
    folders: [...(shown?.folders ?? []), ...shownFolders(prefix, pg.folders ?? [])],
    objects: [...(shown?.objects ?? []), ...(pg.objects ?? [])],
    next: pg.next,
  }
}

const list: List = (p, prefix, after) => BucketService.List(p, prefix, after)

// FilesPanel is the Files tab (SPEC 5.12): one folder of the bucket with a
// path bar back to the top. A folder opens in place; a file opens in the
// main area. It loads 100 entries at a time as the list scrolls. cache/,
// the app's own folder, is listed apart as *App cache*.
export function FilesPanel({ project }: { project: string }) {
  const prefix = useFiles((s) => s.folder[project] ?? "")
  const rev = useFiles((s) => s.rev[project] ?? 0)
  const setFolder = useFiles((s) => s.setFolder)
  const [shown, setShown] = React.useState<{ prefix: string; folder: Folder } | null>(null)
  const [error, setError] = React.useState("")
  const [hasCache, setHasCache] = React.useState(false)
  const [reload, setReload] = React.useState(0)
  const loading = React.useRef(false)
  const top = React.useRef<HTMLDivElement>(null)

  // Another folder starts at its top.
  React.useEffect(() => {
    top.current?.closest('[data-sidebar="content"]')?.scrollTo({ top: 0 })
  }, [prefix])

  React.useEffect(() => {
    let live = true
    setError("")
    listFolder(list, project, prefix, null).then(
      (f) => live && setShown({ prefix, folder: f }),
      (err) => live && setError(toUIError(err).message),
    )
    if (prefix === "") {
      list(project, CACHE, "").then(
        (pg) => live && setHasCache((pg.folders?.length ?? 0) + (pg.objects?.length ?? 0) > 0),
        () => {},
      )
    }
    return () => {
      live = false
    }
  }, [project, prefix, rev, reload])

  const folder = shown?.prefix === prefix ? shown.folder : null
  const more = React.useCallback(() => {
    if (!folder?.next || loading.current) return
    loading.current = true
    listFolder(list, project, prefix, folder, folder.next)
      .then((f) => setShown((s) => (s?.prefix === prefix ? { prefix, folder: f } : s)), (err) => setError(toUIError(err).message))
      .finally(() => (loading.current = false))
  }, [folder, project, prefix])

  return (
    <>
      <SidebarGroup>
        {/* The path bar stays at the top while a long folder scrolls. */}
        <div ref={top} className="sticky top-0 z-10 -mx-2 -mt-2 flex min-h-12 items-center bg-sidebar ps-4 pe-10">
          <PathBar prefix={prefix} onOpen={(p) => setFolder(project, p)} />
          <SidebarGroupAction title="Refresh" onClick={() => setReload((n) => n + 1)}>
            <RefreshCwIcon />
            <span className="sr-only">Refresh</span>
          </SidebarGroupAction>
        </div>
        <SidebarGroupContent>
          {error ? (
            <Note title="Files can't be shown" text={error} />
          ) : !folder ? (
            <SidebarMenu>
              {Array.from({ length: 3 }, (_, i) => (
                <SidebarMenuItem key={i}>
                  <SidebarMenuSkeleton showIcon />
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          ) : folder.folders.length + folder.objects.length === 0 ? (
            prefix === "" ? (
              <Note title="No files yet" text="Files you upload and files the agent saves show here." />
            ) : (
              <Note title="This folder is empty" text="Its files were deleted or moved." />
            )
          ) : (
            <Entries project={project} folder={folder} onFolder={(p) => setFolder(project, p)} onMore={more} />
          )}
        </SidebarGroupContent>
      </SidebarGroup>
      {prefix === "" && hasCache && (
        <SidebarGroup className="mt-auto">
          <SidebarGroupLabel>App</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton size="lg" onClick={() => setFolder(project, CACHE)}>
                  <DatabaseIcon />
                  <div className="grid flex-1 text-start leading-tight">
                    <span className="truncate">App cache</span>
                    <span className="truncate text-xs text-muted-foreground">Fetched pages and tool outputs, kept 30 days</span>
                  </div>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      )}
    </>
  )
}

// PathBar is the way back to the top. Keys are ASCII (SPEC 4.1), but each
// part is isolated so the bar reads the same next to right-to-left text.
function PathBar({ prefix, onOpen }: { prefix: string; onOpen: (prefix: string) => void }) {
  const cs = crumbs(prefix)
  return (
    <Breadcrumb className="min-w-0 py-1">
      <BreadcrumbList className="gap-1 text-xs sm:gap-1">
        {cs.map((c, i) => (
          <React.Fragment key={c.prefix}>
            {i > 0 && <BreadcrumbSeparator />}
            <BreadcrumbItem className="min-w-0">
              {i === cs.length - 1 ? (
                <BreadcrumbPage className="max-w-40 truncate">
                  <bdi>{c.label}</bdi>
                </BreadcrumbPage>
              ) : (
                <BreadcrumbLink asChild>
                  <button type="button" className="max-w-28 truncate" onClick={() => onOpen(c.prefix)}>
                    <bdi>{c.label}</bdi>
                  </button>
                </BreadcrumbLink>
              )}
            </BreadcrumbItem>
          </React.Fragment>
        ))}
      </BreadcrumbList>
    </Breadcrumb>
  )
}

// rowStyle lets the browser skip the layout of rows off screen, so a
// folder of a few thousand files stays quick (SPEC 5.8).
const rowStyle = "[content-visibility:auto] [contain-intrinsic-size:auto_2.5rem]"

function Entries({ project, folder, onFolder, onMore }: { project: string; folder: Folder; onFolder: (prefix: string) => void; onMore: () => void }) {
  const chat = useNav((s) => {
    const p = current(s)
    return p?.view === "chat" && p.project === project ? p.chat : undefined
  })
  const active = useDock((s) => {
    const d = chat ? s.open[chatKey(project, chat)] : undefined
    return d?.kind === "file" ? d.key : undefined
  })
  const end = React.useRef<HTMLLIElement>(null)
  React.useEffect(() => {
    const el = end.current
    if (!el || !folder.next) return
    const io = new IntersectionObserver((es) => es.some((e) => e.isIntersecting) && onMore(), { rootMargin: "400px" })
    io.observe(el)
    return () => io.disconnect()
  }, [folder.next, onMore])
  return (
    <SidebarMenu>
      {folder.folders.map((f) => (
        <SidebarMenuItem key={f} className={rowStyle}>
          <SidebarMenuButton onClick={() => onFolder(f)}>
            <FolderIcon />
            <span className="truncate">{nameOf(f)}</span>
          </SidebarMenuButton>
        </SidebarMenuItem>
      ))}
      {folder.objects.map((o) => (
        <FileRow key={o.key} project={project} obj={o} active={o.key === active} onOpen={chat ? () => openFile(project, chat, o.key) : undefined} />
      ))}
      {folder.next && (
        <SidebarMenuItem ref={end}>
          <SidebarMenuSkeleton showIcon />
        </SidebarMenuItem>
      )}
    </SidebarMenu>
  )
}

function FileRow({ project, obj, active, onOpen }: { project: string; obj: FileObject; active: boolean; onOpen?: () => void }) {
  const kind = fileKind(obj.mime)
  const Icon = kind === "image" ? FileImageIcon : kind === "text" || kind === "markdown" ? FileTextIcon : FileIcon
  return (
    <SidebarMenuItem className={rowStyle}>
      <SidebarMenuButton size={obj.title ? "lg" : "default"} isActive={active} onClick={onOpen} title={obj.key}>
        <Icon />
        <div className="grid flex-1 text-start leading-tight">
          <span className="truncate">{nameOf(obj.key)}</span>
          {obj.title && (
            <span className="truncate text-xs text-muted-foreground" dir="auto">
              {obj.title}
            </span>
          )}
        </div>
      </SidebarMenuButton>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <SidebarMenuAction showOnHover>
            <MoreHorizontalIcon />
            <span className="sr-only">More</span>
          </SidebarMenuAction>
        </DropdownMenuTrigger>
        <DropdownMenuContent side="left" align="start" className="w-56">
          <DropdownMenuGroup>
            <DropdownMenuItem onSelect={() => copyLink(project, obj.key)}>
              <LinkIcon />
              Copy link
            </DropdownMenuItem>
            {obj.opens && (
              <DropdownMenuItem onSelect={() => void openWithSystem(project, obj.key)}>
                <ExternalLinkIcon />
                Open with the system app
              </DropdownMenuItem>
            )}
          </DropdownMenuGroup>
          <DropdownMenuSeparator />
          <DropdownMenuGroup>
            <DropdownMenuItem variant="destructive" onSelect={() => void deleteFile(project, obj.key)}>
              <Trash2Icon />
              Delete
            </DropdownMenuItem>
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>
    </SidebarMenuItem>
  )
}

function Note({ title, text }: { title: string; text: string }) {
  return (
    <Empty className="p-4">
      <EmptyHeader>
        <EmptyTitle className="text-sm">{title}</EmptyTitle>
        <EmptyDescription>{text}</EmptyDescription>
      </EmptyHeader>
    </Empty>
  )
}
