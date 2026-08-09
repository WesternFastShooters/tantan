import type { ComponentProps } from "react"
import { Link, useLocation } from "react-router"

import type { HomeCard } from "~/lib/tantan-api/gen/types"

interface EntryLinkProps extends Omit<ComponentProps<typeof Link>, "to" | "state"> {
  card: HomeCard
}

export function EntryLink({ card, children, ...props }: EntryLinkProps) {
  const location = useLocation()
  return (
    <Link
      {...props}
      to={`/entries/${encodeURIComponent(card.entryId)}`}
      state={{ card, returnTo: `${location.pathname}${location.search}` }}
    >
      {children}
    </Link>
  )
}
