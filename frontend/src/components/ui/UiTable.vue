<script setup lang="ts">
interface Props {
  caption?: string
  stickyHeader?: boolean
  dense?: boolean
  empty?: boolean
  emptyText?: string
  /**
   * When true, under the small breakpoint the table collapses to stacked cards.
   * Each td needs a `data-label="Column"` attribute so the mobile fallback can
   * render the column header next to the value.
   * Use on summary / KPI-style tables (§5 of PMS_08). Leave off for ledger /
   * detail tables that should keep horizontal scroll on mobile.
   */
  stack?: boolean
}

withDefaults(defineProps<Props>(), {
  caption: '',
  stickyHeader: false,
  dense: false,
  empty: false,
  emptyText: 'No records',
  stack: false,
})
</script>

<template>
  <div class="ui-table-wrap" :class="{ 'ui-table-wrap--stack': stack }">
    <table
      class="ui-table"
      :class="{
        'ui-table--sticky': stickyHeader,
        'ui-table--dense': dense,
        'ui-table--stack': stack,
      }"
      :data-stack="stack ? 'true' : undefined"
    >
      <caption v-if="caption" class="ui-table__caption">{{ caption }}</caption>
      <thead v-if="$slots.head">
        <slot name="head" />
      </thead>
      <tbody>
        <slot />
        <tr v-if="empty" class="ui-table__empty-row">
          <td :colspan="100">{{ emptyText }}</td>
        </tr>
      </tbody>
      <tfoot v-if="$slots.foot">
        <slot name="foot" />
      </tfoot>
    </table>
  </div>
</template>

<style scoped>
.ui-table-wrap {
  overflow-x: auto;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  background: var(--color-surface);
}
.ui-table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--font-size-sm);
}
.ui-table__caption {
  text-align: left;
  padding: var(--space-3) var(--space-4);
  color: var(--color-text-muted);
  font-size: var(--font-size-xs);
  border-bottom: 1px solid var(--color-border);
}
.ui-table :deep(thead th) {
  text-align: left;
  padding: var(--space-3) var(--space-4);
  font-weight: 600;
  color: var(--color-text-muted);
  background: var(--color-sunken);
  border-bottom: 1px solid var(--color-border);
  white-space: nowrap;
}
.ui-table--sticky :deep(thead th) {
  position: sticky;
  top: 0;
  z-index: 1;
}
.ui-table :deep(tbody td) {
  padding: var(--space-3) var(--space-4);
  border-bottom: 1px solid var(--color-border);
  color: var(--color-text);
  vertical-align: middle;
}
.ui-table :deep(tbody tr:last-child td) {
  border-bottom: none;
}
.ui-table :deep(tbody tr:hover td) {
  background: var(--color-sunken);
}
.ui-table--dense :deep(thead th),
.ui-table--dense :deep(tbody td) {
  padding: var(--space-2) var(--space-3);
}
.ui-table :deep(td.num),
.ui-table :deep(th.num) {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.ui-table__empty-row td {
  text-align: center;
  color: var(--color-text-muted);
  padding: var(--space-5) var(--space-4);
  font-style: italic;
}

/*
 * Stacked fallback for summary / KPI tables on narrow viewports.
 * Consumers must set data-label="Column name" on each <td> to render the
 * column header prefix. Headers are hidden, each row becomes a card.
 */
@media (max-width: 639.98px) {
  .ui-table-wrap--stack {
    overflow-x: visible;
    border: none;
    background: transparent;
    border-radius: 0;
  }
  .ui-table--stack :deep(thead) {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0 0 0 0);
    white-space: nowrap;
  }
  .ui-table--stack :deep(tbody tr) {
    display: block;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    padding: var(--space-2) var(--space-3);
    margin-bottom: var(--space-3);
  }
  .ui-table--stack :deep(tbody tr:hover td) {
    background: transparent;
  }
  .ui-table--stack :deep(tbody td) {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: var(--space-3);
    padding: var(--space-1) 0;
    border-bottom: none;
  }
  .ui-table--stack :deep(tbody td::before) {
    content: attr(data-label);
    flex: 0 0 40%;
    color: var(--color-text-muted);
    font-size: var(--font-size-xs);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    font-weight: 600;
  }
  .ui-table--stack :deep(tbody td:not([data-label])::before) {
    content: none;
  }
}
</style>
