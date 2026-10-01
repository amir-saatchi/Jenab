import * as React from "react"
import { BellIcon, CheckIcon, ChevronRightIcon, ClipboardPasteIcon, KeyRoundIcon } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group"
import { Item, ItemContent, ItemMedia, ItemTitle } from "@/components/ui/item"
import {
  Questionnaire,
  QuestionnaireActions,
  QuestionnaireChoice,
  QuestionnaireChoiceDescription,
  QuestionnaireChoices,
  QuestionnaireInput,
  QuestionnaireItem,
  QuestionnaireProgress,
  QuestionnaireSubmit,
  QuestionnaireTitle,
} from "@/components/ui/questionnaire"
import { cn } from "@/lib/utils"

// Every approval is one card (SPEC 8.8): what, why, the risk, details, and the choice.

function Kicker({ children }: { children: React.ReactNode }) {
  return (
    <span className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
      {children}
    </span>
  )
}

export function ApprovalCard({
  kind,
  title,
  why,
  risk,
  details,
  detailsOpen,
  allow = "Allow",
  note,
  className,
}: {
  kind: string
  title: string
  why: string
  risk?: string
  details?: React.ReactNode
  detailsOpen?: boolean
  allow?: string
  note?: boolean
  className?: string
}) {
  return (
    <Card size="sm" className={cn("not-typeset", className)}>
      <CardHeader>
        <Kicker>{kind}</Kicker>
        <CardTitle className="text-base">{title}</CardTitle>
        <CardDescription className="flex flex-col gap-1">
          <span className="text-foreground" dir="auto">
            {why}
          </span>
          {risk && <span>{risk}</span>}
        </CardDescription>
      </CardHeader>
      {details && (
        <CardContent>
          <Collapsible defaultOpen={detailsOpen} className="flex flex-col gap-2">
            <CollapsibleTrigger asChild>
              <Button variant="ghost" size="xs" className="w-fit text-muted-foreground">
                <ChevronRightIcon
                  data-icon="inline-start"
                  className="transition-transform group-data-[state=open]/button:rotate-90"
                />
                Details
              </Button>
            </CollapsibleTrigger>
            <CollapsibleContent>{details}</CollapsibleContent>
          </Collapsible>
        </CardContent>
      )}
      <CardFooter className="gap-2">
        <Button size="sm">{allow}</Button>
        <Button size="sm" variant="outline">
          Deny
        </Button>
        {note && <Input className="h-7 flex-1 text-sm" placeholder="Note for the agent (optional)" />}
      </CardFooter>
    </Card>
  )
}

export function DetailList({ rows }: { rows: [string, React.ReactNode, boolean?][] }) {
  return (
    <dl className="grid grid-cols-[7rem_1fr] gap-x-4 gap-y-1.5 rounded-lg bg-muted/60 p-3 text-sm">
      {rows.map(([k, v, danger]) => (
        <React.Fragment key={k}>
          <dt className={cn("text-muted-foreground", danger && "text-tone-red")}>{k}</dt>
          <dd className={cn(danger && "text-tone-red")}>{v}</dd>
        </React.Fragment>
      ))}
    </dl>
  )
}

export function ApprovalRecord({ children }: { children: React.ReactNode }) {
  return (
    <Item variant="outline" size="xs" className="w-fit not-typeset">
      <ItemMedia>
        <CheckIcon className="size-4 text-tone-green" />
      </ItemMedia>
      <ItemContent>
        <ItemTitle className="font-normal text-muted-foreground">{children}</ItemTitle>
      </ItemContent>
    </Item>
  )
}

export function ConnectionCard() {
  return (
    <Card size="sm" className="not-typeset">
      <CardHeader>
        <Kicker>Connection</Kicker>
        <CardTitle className="text-base">Save the CoinGecko connection?</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <dl className="grid grid-cols-[7rem_1fr] gap-x-4 gap-y-1.5 text-sm">
          <dt className="text-muted-foreground">Title</dt>
          <dd>CoinGecko</dd>
          <dt className="text-muted-foreground">Host</dt>
          <dd>api.coingecko.com</dd>
          <dt className="text-muted-foreground">Key goes in</dt>
          <dd className="font-mono text-xs leading-5">header x-cg-demo-api-key</dd>
        </dl>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="cg-key">API key</FieldLabel>
            <InputGroup>
              <InputGroupAddon>
                <KeyRoundIcon />
              </InputGroupAddon>
              <InputGroupInput id="cg-key" type="password" defaultValue="cg-demo-0000000000" />
              <InputGroupAddon align="inline-end">
                <InputGroupButton>
                  <ClipboardPasteIcon />
                  Paste
                </InputGroupButton>
              </InputGroupAddon>
            </InputGroup>
            <FieldDescription>Stored in the OS keychain. The agent never sees it.</FieldDescription>
          </Field>
        </FieldGroup>
      </CardContent>
      <CardFooter className="gap-2">
        <Button size="sm">Save</Button>
        <Button size="sm" variant="outline">
          Cancel
        </Button>
      </CardFooter>
    </Card>
  )
}

const currencyItems = [
  { name: "currency", required: true, choices: [{ value: "eur" }, { value: "usd" }] },
] as const

// ask_user (SPEC 8.8): 1–4 questions, 2–4 options, one may be recommended, Other is always there.
export function QuestionForm() {
  return (
    <Questionnaire defaultItem="currency" items={currencyItems} shortcuts="numbers" className="not-typeset">
      <QuestionnaireItem name="currency" required>
        <Card size="sm">
          <CardHeader>
            <Kicker>Question</Kicker>
            <QuestionnaireTitle render={<CardTitle className="text-base" />}>
              Which currency should prices use?
            </QuestionnaireTitle>
          </CardHeader>
          <CardContent>
            <QuestionnaireChoices>
              <QuestionnaireChoice value="eur" defaultChecked>
                <span className="flex items-center gap-2 font-medium">
                  EUR <Badge variant="secondary">Recommended</Badge>
                </span>
                <QuestionnaireChoiceDescription>Converted from USD every day</QuestionnaireChoiceDescription>
              </QuestionnaireChoice>
              <QuestionnaireChoice value="usd">
                <span className="font-medium">USD</span>
                <QuestionnaireChoiceDescription>As the price API returns them</QuestionnaireChoiceDescription>
              </QuestionnaireChoice>
              <QuestionnaireInput aria-label="Other answer" placeholder="Other…" dir="auto" />
            </QuestionnaireChoices>
          </CardContent>
          <CardFooter className="flex items-center gap-3">
            <QuestionnaireActions className="w-full">
              <span className="col-span-2 self-center text-sm text-muted-foreground">
                Or type your answer in the chat.
              </span>
              <QuestionnaireProgress className="hidden" />
              <QuestionnaireSubmit size="sm">Submit</QuestionnaireSubmit>
            </QuestionnaireActions>
          </CardFooter>
        </Card>
      </QuestionnaireItem>
    </Questionnaire>
  )
}

// Shown above the composer while a card waits out of view (SPEC 8.8).
export function WaitingBar({ children }: { children: React.ReactNode }) {
  return (
    <Item variant="muted" size="xs" className="not-typeset">
      <ItemMedia>
        <BellIcon className="size-4 text-tone-amber" />
      </ItemMedia>
      <ItemContent>
        <ItemTitle className="font-normal">{children}</ItemTitle>
      </ItemContent>
      <Button size="xs" variant="outline">
        Show
      </Button>
    </Item>
  )
}

export function DesktopNotification() {
  return (
    <Card size="sm" className="w-80 shadow-lg">
      <CardHeader>
        <CardDescription className="flex items-center gap-2 text-xs">
          <span className="flex size-4 items-center justify-center rounded bg-primary text-[0.6rem] font-bold text-primary-foreground">
            B
          </span>
          Jenab · Bitcoin
        </CardDescription>
        <CardTitle>Mother is waiting for your approval</CardTitle>
        <CardDescription>Allow api.coingecko.com?</CardDescription>
      </CardHeader>
    </Card>
  )
}
