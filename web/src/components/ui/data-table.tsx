"use client";

import {
  flexRender,
  getCoreRowModel,
  getFacetedRowModel,
  getFacetedUniqueValues,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
  type ColumnDef,
  type ColumnFiltersState,
  type FilterFn,
  type Row,
  type RowSelectionState,
  type SortingState,
  type VisibilityState,
} from "@tanstack/react-table";
import { ContextMenu as CM, DropdownMenu as DM } from "radix-ui";
import {
  ArrowDown,
  ArrowUp,
  BookmarkPlus,
  ChevronLeft,
  ChevronRight,
  ChevronsUpDown,
  Columns3,
  ListFilter,
  MoreHorizontal,
  Search,
  SearchX,
  X,
  type LucideIcon,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState, type KeyboardEvent, type MouseEvent, type ReactNode } from "react";
import { cn } from "@/lib/cn";
import { Button } from "./button";
import { EmptyState } from "./empty-state";
import { Checkbox, Input } from "./input";
import { Kbd } from "./kbd";
import { Menu, MenuCheckboxItem, MenuContent, MenuLabel, MenuSeparator, MenuTrigger } from "./menu";

declare module "@tanstack/react-table" {
  interface ColumnMeta<TData, TValue> {
    label: string;
    align?: "left" | "right";
    hideable?: boolean;
    className?: string;
  }
}

export const facetFilter: FilterFn<any> = (row, columnId, value: string[]) => {
  if (!value || value.length === 0) return true;
  const cell = row.getValue<string | string[]>(columnId);
  return Array.isArray(cell) ? cell.some((item) => value.includes(item)) : value.includes(cell);
};

export type Facet = { column: string; label: string; options: { value: string; label: string }[] };
export type SavedView = { id: string; label: string; filters: ColumnFiltersState; custom?: boolean };
export type RowAction<T> = { label: string; icon?: LucideIcon; danger?: boolean; separatorBefore?: boolean; onSelect: (row: T) => void };
export type BulkAction<T> = { label: string; icon?: LucideIcon; danger?: boolean; onSelect: (rows: T[]) => void };

const sameFilters = (a: ColumnFiltersState, b: ColumnFiltersState) => {
  const norm = (f: ColumnFiltersState) =>
    JSON.stringify(
      [...f]
        .filter((x) => !(Array.isArray(x.value) && x.value.length === 0))
        .map((x) => ({ id: x.id, value: Array.isArray(x.value) ? [...x.value].sort() : x.value }))
        .sort((p, q) => p.id.localeCompare(q.id)),
    );
  return norm(a) === norm(b);
};

export function DataTable<T>({
  data,
  columns,
  getRowId,
  label,
  noun,
  storageKey,
  searchText,
  searchPlaceholder,
  facets = [],
  views = [],
  initialViewId,
  initialVisibility = {},
  rowActions,
  bulkActions,
  onRowOpen,
  toolbarEnd,
  pageSize = 25,
}: {
  data: T[];
  columns: ColumnDef<T, any>[];
  getRowId: (row: T) => string;
  label: string;
  noun: [string, string];
  storageKey: string;
  searchText: (row: T) => string;
  searchPlaceholder: string;
  facets?: Facet[];
  views?: SavedView[];
  initialViewId?: string;
  initialVisibility?: VisibilityState;
  rowActions?: RowAction<T>[];
  bulkActions?: BulkAction<T>[];
  onRowOpen?: (row: T) => void;
  toolbarEnd?: ReactNode | ((rows: T[]) => ReactNode);
  pageSize?: number;
}) {
  const [sorting, setSorting] = useState<SortingState>([]);
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>(
    (views.find((view) => view.id === initialViewId) ?? views[0])?.filters ?? [],
  );
  const [globalFilter, setGlobalFilter] = useState("");
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>(initialVisibility);
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({});
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize });
  const [customViews, setCustomViews] = useState<SavedView[]>([]);
  const [naming, setNaming] = useState(false);
  const [viewName, setViewName] = useState("");
  const searchRef = useRef<HTMLInputElement>(null);
  const bodyRef = useRef<HTMLTableSectionElement>(null);
  const viewsKey = `halo.views.${storageKey}`;

  useEffect(() => {
    try {
      const stored = localStorage.getItem(viewsKey);
      if (stored) setCustomViews(JSON.parse(stored) as SavedView[]);
    } catch {
      setCustomViews([]);
    }
  }, [viewsKey]);

  useEffect(() => {
    function onKey(event: globalThis.KeyboardEvent) {
      const target = event.target as HTMLElement | null;
      const typing = target && (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable);
      if (event.key === "/" && !typing) {
        event.preventDefault();
        searchRef.current?.focus();
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const allColumns = useMemo<ColumnDef<T, any>[]>(() => {
    const select: ColumnDef<T, any> = {
      id: "_select",
      enableSorting: false,
      enableHiding: false,
      meta: { label: "Select", className: "w-12" },
      header: ({ table }) => (
        <Checkbox
          aria-label={`Select all ${noun[1]} on this page`}
          checked={table.getIsAllPageRowsSelected()}
          ref={(el) => {
            if (el) el.indeterminate = table.getIsSomePageRowsSelected();
          }}
          onChange={table.getToggleAllPageRowsSelectedHandler()}
        />
      ),
      cell: ({ row }) => (
        <Checkbox
          aria-label="Select row"
          checked={row.getIsSelected()}
          onChange={row.getToggleSelectedHandler()}
          onClick={(event) => event.stopPropagation()}
        />
      ),
    };
    const actions: ColumnDef<T, any> = {
      id: "_actions",
      enableSorting: false,
      enableHiding: false,
      meta: { label: "Actions", className: "w-12", align: "right" },
      header: () => <span className="sr-only">Actions</span>,
      cell: ({ row }) => (rowActions ? <RowMenu row={row.original} actions={rowActions} /> : null),
    };
    return [...(bulkActions ? [select] : []), ...columns, ...(rowActions ? [actions] : [])];
  }, [columns, bulkActions, rowActions, noun]);

  const table = useReactTable({
    data,
    columns: allColumns,
    getRowId,
    state: { sorting, columnFilters, globalFilter, columnVisibility, rowSelection, pagination },
    onSortingChange: setSorting,
    onColumnFiltersChange: setColumnFilters,
    onGlobalFilterChange: setGlobalFilter,
    onColumnVisibilityChange: setColumnVisibility,
    onRowSelectionChange: setRowSelection,
    onPaginationChange: setPagination,
    globalFilterFn: (row, _columnId, value: string) => searchText(row.original).toLowerCase().includes(value.trim().toLowerCase()),
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getFacetedRowModel: getFacetedRowModel(),
    getFacetedUniqueValues: getFacetedUniqueValues(),
  });

  const filteredCount = table.getFilteredRowModel().rows.length;
  const selected = table.getSelectedRowModel().rows.map((row) => row.original);
  const rows = table.getRowModel().rows;
  const allViews = [...views, ...customViews];
  const activeView = allViews.find((view) => sameFilters(view.filters, columnFilters));
  const hasFilters = columnFilters.some((f) => !(Array.isArray(f.value) && f.value.length === 0)) || globalFilter.trim() !== "";
  const start = filteredCount === 0 ? 0 : pagination.pageIndex * pagination.pageSize + 1;
  const end = Math.min(filteredCount, (pagination.pageIndex + 1) * pagination.pageSize);

  function applyView(view: SavedView) {
    setColumnFilters(view.filters);
    setRowSelection({});
  }

  function saveView() {
    const label = viewName.trim();
    if (!label) return;
    const next = [...customViews, { id: `custom-${Date.now()}`, label, filters: columnFilters, custom: true }];
    setCustomViews(next);
    try {
      localStorage.setItem(viewsKey, JSON.stringify(next));
    } catch {}
    setNaming(false);
    setViewName("");
  }

  function removeView(id: string) {
    const next = customViews.filter((view) => view.id !== id);
    setCustomViews(next);
    try {
      localStorage.setItem(viewsKey, JSON.stringify(next));
    } catch {}
  }

  function clearFilters() {
    setColumnFilters([]);
    setGlobalFilter("");
  }

  function focusRow(event: KeyboardEvent<HTMLTableRowElement>, delta: number) {
    const rowsEls = Array.from(bodyRef.current?.querySelectorAll<HTMLTableRowElement>("tr[data-row]") ?? []);
    const next = rowsEls[rowsEls.indexOf(event.currentTarget) + delta];
    if (next) {
      event.preventDefault();
      next.focus();
    }
  }

  function onRowKeyDown(event: KeyboardEvent<HTMLTableRowElement>, row: Row<T>) {
    if (event.target !== event.currentTarget) return;
    if (event.key === "ArrowDown" || event.key === "j") focusRow(event, 1);
    else if (event.key === "ArrowUp" || event.key === "k") focusRow(event, -1);
    else if (event.key === "Enter" && onRowOpen) {
      event.preventDefault();
      onRowOpen(row.original);
    } else if (event.key === "x" && bulkActions) {
      event.preventDefault();
      row.toggleSelected();
    }
  }

  const hideable = table.getAllLeafColumns().filter((column) => column.getCanHide() && column.columnDef.meta?.hideable !== false);

  return (
    <div className="flex flex-col gap-4">
      {allViews.length > 0 ? (
        <div className="-mx-1 flex items-center gap-1 overflow-x-auto px-1" role="toolbar" aria-label="Saved views">
          {allViews.map((view) => {
            const current = activeView?.id === view.id;
            return (
              <span key={view.id} className="group relative inline-flex shrink-0">
                <button
                  type="button"
                  onClick={() => applyView(view)}
                  aria-pressed={current}
                  className={cn(
                    "h-8 rounded-md px-3 text-body-sm font-medium transition-colors duration-fast ease-brand",
                    current ? "bg-press text-fg" : "text-fg-3 hover:bg-hover hover:text-fg",
                    view.custom && "pr-8",
                  )}
                >
                  {view.label}
                </button>
                {view.custom ? (
                  <button
                    type="button"
                    onClick={() => removeView(view.id)}
                    aria-label={`Delete view ${view.label}`}
                    className="absolute top-1/2 right-1 grid size-6 -translate-y-1/2 place-items-center rounded-sm text-fg-3 hover:bg-press hover:text-fg"
                  >
                    <X aria-hidden="true" size={16} strokeWidth={1.75} />
                  </button>
                ) : null}
              </span>
            );
          })}
          {!activeView && hasFilters ? (
            naming ? (
              <form
                className="flex shrink-0 items-center gap-2"
                onSubmit={(event) => {
                  event.preventDefault();
                  saveView();
                }}
              >
                <Input
                  size="sm"
                  autoFocus
                  value={viewName}
                  onChange={(event) => setViewName(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key === "Escape") setNaming(false);
                  }}
                  placeholder="View name"
                  aria-label="View name"
                  className="w-44"
                />
                <Button type="submit" size="sm" variant="secondary" disabled={!viewName.trim()}>
                  Save
                </Button>
              </form>
            ) : (
              <button
                type="button"
                onClick={() => setNaming(true)}
                className="inline-flex h-8 shrink-0 items-center gap-2 rounded-md px-3 text-body-sm font-medium text-fg-3 transition-colors duration-fast hover:bg-hover hover:text-fg"
              >
                <BookmarkPlus aria-hidden="true" size={16} strokeWidth={1.75} />
                Save view
              </button>
            )
          ) : null}
        </div>
      ) : null}

      <div className="flex flex-wrap items-center gap-2">
        <div className="w-full sm:w-72">
          <Input
            ref={searchRef}
            size="sm"
            type="search"
            value={globalFilter}
            onChange={(event) => setGlobalFilter(event.target.value)}
            placeholder={searchPlaceholder}
            aria-label={searchPlaceholder}
            leading={<Search size={16} strokeWidth={1.75} />}
            trailing={globalFilter ? null : <Kbd className="max-sm:hidden">/</Kbd>}
          />
        </div>
        {facets.map((facet) => (
          <FacetMenu
            key={facet.column}
            facet={facet}
            counts={table.getColumn(facet.column)?.getFacetedUniqueValues()}
            value={(columnFilters.find((f) => f.id === facet.column)?.value as string[] | undefined) ?? []}
            onChange={(value) =>
              setColumnFilters((current) => [...current.filter((f) => f.id !== facet.column), ...(value.length ? [{ id: facet.column, value }] : [])])
            }
          />
        ))}
        {hasFilters ? (
          <Button size="sm" variant="quiet" onClick={clearFilters}>
            Clear
          </Button>
        ) : null}
        <div className="ml-auto flex items-center gap-2">
          {typeof toolbarEnd === "function" ? toolbarEnd(table.getFilteredRowModel().rows.map((row) => row.original)) : toolbarEnd}
          {hideable.length > 0 ? (
            <Menu>
              <MenuTrigger asChild>
                <Button size="sm" variant="quiet" aria-label="Choose columns">
                  <Columns3 aria-hidden="true" size={16} strokeWidth={1.75} />
                  <span className="hidden md:inline">Columns</span>
                </Button>
              </MenuTrigger>
              <MenuContent>
                <MenuLabel>Visible columns</MenuLabel>
                {hideable.map((column) => (
                  <MenuCheckboxItem key={column.id} checked={column.getIsVisible()} onCheckedChange={(value) => column.toggleVisibility(value)}>
                    {column.columnDef.meta?.label ?? column.id}
                  </MenuCheckboxItem>
                ))}
              </MenuContent>
            </Menu>
          ) : null}
        </div>
      </div>

      <div className="overflow-hidden rounded-lg border border-border bg-surface">
        <div className="relative overflow-x-auto">
          <table className="w-full border-collapse text-left text-body-sm">
            <caption className="sr-only">{label}</caption>
            <thead>
              {table.getHeaderGroups().map((group) => (
                <tr key={group.id} className="border-b border-border">
                  {group.headers.map((header) => {
                    const meta = header.column.columnDef.meta;
                    const sorted = header.column.getIsSorted();
                    return (
                      <th
                        key={header.id}
                        scope="col"
                        aria-sort={sorted === "asc" ? "ascending" : sorted === "desc" ? "descending" : undefined}
                        className={cn(
                          "h-10 px-3 text-label whitespace-nowrap text-fg-3 first:pl-4 last:pr-4",
                          meta?.align === "right" && "text-right",
                          meta?.className,
                        )}
                      >
                        {header.isPlaceholder ? null : header.column.getCanSort() ? (
                          <button
                            type="button"
                            onClick={header.column.getToggleSortingHandler()}
                            className={cn(
                              "-mx-1 inline-flex items-center gap-1 rounded-sm px-1 transition-colors duration-fast hover:text-fg",
                              sorted && "text-fg",
                              meta?.align === "right" && "flex-row-reverse",
                            )}
                          >
                            {flexRender(header.column.columnDef.header, header.getContext())}
                            {sorted === "asc" ? (
                              <ArrowUp aria-hidden="true" size={16} strokeWidth={1.75} />
                            ) : sorted === "desc" ? (
                              <ArrowDown aria-hidden="true" size={16} strokeWidth={1.75} />
                            ) : (
                              <ChevronsUpDown aria-hidden="true" size={16} strokeWidth={1.75} className="text-n600" />
                            )}
                          </button>
                        ) : (
                          flexRender(header.column.columnDef.header, header.getContext())
                        )}
                      </th>
                    );
                  })}
                </tr>
              ))}
            </thead>
            <tbody ref={bodyRef}>
              {rows.map((row) => {
                const tr = (
                  <tr
                    key={row.id}
                    data-row=""
                    tabIndex={0}
                    aria-selected={bulkActions ? row.getIsSelected() : undefined}
                    onClick={onRowOpen ? () => onRowOpen(row.original) : undefined}
                    onKeyDown={(event) => onRowKeyDown(event, row)}
                    className={cn(
                      "h-12 border-b border-border outline-none last:border-b-0 transition-colors duration-fast ease-brand",
                      "focus-visible:bg-press focus-visible:shadow-[inset_2px_0_0_var(--color-ember)]",
                      row.getIsSelected() ? "bg-press" : "hover:bg-hover",
                      onRowOpen && "cursor-pointer",
                    )}
                  >
                    {row.getVisibleCells().map((cell) => {
                      const meta = cell.column.columnDef.meta;
                      return (
                        <td
                          key={cell.id}
                          className={cn("px-3 align-middle text-fg-2 first:pl-4 last:pr-4", meta?.align === "right" && "text-right", meta?.className)}
                        >
                          {flexRender(cell.column.columnDef.cell, cell.getContext())}
                        </td>
                      );
                    })}
                  </tr>
                );
                return rowActions ? (
                  <CM.Root key={row.id}>
                    <CM.Trigger asChild>{tr}</CM.Trigger>
                    <CM.Portal>
                      <CM.Content className="z-50 min-w-48 rounded-md border border-border bg-surface p-1 shadow-md outline-none data-[state=open]:animate-[halo-pop_160ms_var(--ease-brand)]">
                        {rowActions.map((action) => (
                          <ActionItem key={action.label} kind="context" action={action} row={row.original} />
                        ))}
                      </CM.Content>
                    </CM.Portal>
                  </CM.Root>
                ) : (
                  tr
                );
              })}
            </tbody>
          </table>
        </div>
        {rows.length === 0 ? (
          <EmptyState
            icon={SearchX}
            title={`No ${noun[1]} match`}
            description="Try a different search, or clear the filters to see everything."
            action={
              <Button size="sm" variant="secondary" onClick={clearFilters}>
                Clear filters
              </Button>
            }
          />
        ) : null}
        <div className="flex flex-wrap items-center justify-between gap-4 border-t border-border px-4 py-2 text-body-sm text-fg-3">
          <span className="tnum">
            {filteredCount === data.length
              ? `${start}–${end} of ${data.length} ${data.length === 1 ? noun[0] : noun[1]}`
              : `${start}–${end} of ${filteredCount} matching · ${data.length} total`}
          </span>
          <div className="flex items-center gap-1">
            <span className="tnum mr-2 hidden sm:inline">
              Page {table.getPageCount() === 0 ? 0 : pagination.pageIndex + 1} of {table.getPageCount()}
            </span>
            <Button size="icon-sm" variant="quiet" aria-label="Previous page" disabled={!table.getCanPreviousPage()} onClick={() => table.previousPage()}>
              <ChevronLeft aria-hidden="true" size={16} strokeWidth={1.75} />
            </Button>
            <Button size="icon-sm" variant="quiet" aria-label="Next page" disabled={!table.getCanNextPage()} onClick={() => table.nextPage()}>
              <ChevronRight aria-hidden="true" size={16} strokeWidth={1.75} />
            </Button>
          </div>
        </div>
      </div>

      {bulkActions && selected.length > 0 ? (
        <div
          role="region"
          aria-label="Bulk actions"
          className="fixed bottom-6 left-1/2 z-30 flex max-w-[calc(100vw-32px)] -translate-x-1/2 animate-enter items-center gap-2 overflow-x-auto rounded-lg border border-border-strong bg-surface p-2 pl-4 shadow-md lg:left-[calc(50%+132px)]"
        >
          <span className="tnum shrink-0 pr-2 text-body-sm font-medium text-fg">{selected.length} selected</span>
          {bulkActions.map((action) => {
            const Icon = action.icon;
            return (
              <Button
                key={action.label}
                size="sm"
                variant={action.danger ? "quiet" : "secondary"}
                className={action.danger ? "text-danger! hover:text-danger!" : undefined}
                onClick={() => {
                  action.onSelect(selected);
                  setRowSelection({});
                }}
              >
                {Icon ? <Icon aria-hidden="true" size={16} strokeWidth={1.75} /> : null}
                {action.label}
              </Button>
            );
          })}
          <Button size="icon-sm" variant="quiet" aria-label="Clear selection" onClick={() => setRowSelection({})}>
            <X aria-hidden="true" size={16} strokeWidth={1.75} />
          </Button>
        </div>
      ) : null}
    </div>
  );
}

function FacetMenu({
  facet,
  counts,
  value,
  onChange,
}: {
  facet: Facet;
  counts?: Map<unknown, number>;
  value: string[];
  onChange: (value: string[]) => void;
}) {
  const summary = value.length === 0 ? null : value.length === 1 ? facet.options.find((o) => o.value === value[0])?.label : `${value.length} selected`;
  return (
    <Menu>
      <MenuTrigger asChild>
        <button
          type="button"
          className={cn(
            "inline-flex h-8 items-center gap-2 rounded-md border px-3 text-body-sm font-medium transition-colors duration-fast ease-brand",
            value.length ? "border-ember/40 bg-ghost text-fg" : "border-border text-fg-3 hover:border-border-strong hover:text-fg",
          )}
        >
          <ListFilter aria-hidden="true" size={16} strokeWidth={1.75} className={value.length ? "text-ember" : undefined} />
          {facet.label}
          {summary ? <span className="text-fg-2">· {summary}</span> : null}
        </button>
      </MenuTrigger>
      <MenuContent align="start">
        <MenuLabel>{facet.label}</MenuLabel>
        {facet.options.map((option) => (
          <MenuCheckboxItem
            key={option.value}
            checked={value.includes(option.value)}
            onCheckedChange={(checked) => onChange(checked ? [...value, option.value] : value.filter((v) => v !== option.value))}
          >
            <span className="flex flex-1 items-center justify-between gap-6">
              {option.label}
              <span className="tnum text-caption text-fg-3">{counts?.get(option.value) ?? 0}</span>
            </span>
          </MenuCheckboxItem>
        ))}
        {value.length ? (
          <>
            <MenuSeparator />
            <DM.Item
              onSelect={() => onChange([])}
              className="flex h-8 cursor-pointer items-center rounded-sm px-2 text-body-sm text-fg-3 outline-none data-highlighted:bg-press data-highlighted:text-fg"
            >
              Clear {facet.label.toLowerCase()}
            </DM.Item>
          </>
        ) : null}
      </MenuContent>
    </Menu>
  );
}

function RowMenu<T>({ row, actions }: { row: T; actions: RowAction<T>[] }) {
  return (
    <Menu>
      <MenuTrigger asChild>
        <Button size="icon-sm" variant="quiet" aria-label="Row actions" onClick={(event) => event.stopPropagation()} onKeyDown={(event) => event.stopPropagation()}>
          <MoreHorizontal aria-hidden="true" size={16} strokeWidth={1.75} />
        </Button>
      </MenuTrigger>
      <MenuContent>
        {actions.map((action) => (
          <ActionItem key={action.label} kind="dropdown" action={action} row={row} />
        ))}
      </MenuContent>
    </Menu>
  );
}

function ActionItem<T>({ kind, action, row }: { kind: "dropdown" | "context"; action: RowAction<T>; row: T }) {
  const Icon = action.icon;
  const Item = kind === "dropdown" ? DM.Item : CM.Item;
  const Separator = kind === "dropdown" ? DM.Separator : CM.Separator;
  return (
    <>
      {action.separatorBefore ? <Separator className="my-1 h-px bg-border" /> : null}
      <Item
        onSelect={() => action.onSelect(row)}
        onClick={(event: MouseEvent) => event.stopPropagation()}
        className={cn(
          "flex h-8 cursor-pointer items-center gap-2 rounded-sm px-2 text-body-sm outline-none select-none data-highlighted:bg-press",
          action.danger ? "text-danger" : "text-fg-2 data-highlighted:text-fg",
        )}
      >
        {Icon ? <Icon aria-hidden="true" size={16} strokeWidth={1.75} className={action.danger ? "text-danger" : "text-fg-3"} /> : null}
        {action.label}
      </Item>
    </>
  );
}
