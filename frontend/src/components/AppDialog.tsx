import type {ReactNode} from 'react'
import {Modal, useOverlayState} from '@heroui/react'
import type {ModalContainerProps} from '@heroui/react'

export function AppDialog({
  isOpen,
  onOpenChange,
  children,
  size = 'lg',
  dialogClassName,
}: {
  isOpen: boolean
  onOpenChange: (isOpen: boolean) => void
  children: ReactNode
  size?: ModalContainerProps['size']
  /** Extra classes on the dialog surface — e.g. a wider max-width than the
   *  HeroUI size presets (sm/md/lg cap at 24/28/32rem). Needs the important
   *  modifier to beat the unlayered HeroUI variant CSS. */
  dialogClassName?: string
}) {
  const state = useOverlayState({isOpen, onOpenChange})

  return (
    <Modal.Root state={state}>
      <Modal.Backdrop className="bg-zinc-950/45 p-3 backdrop-blur-sm dark:bg-black/65">
        <Modal.Container
          size={size}
          scroll="inside"
          placement="center"
          className="max-h-[calc(100dvh-1.5rem)] w-full"
        >
          <Modal.Dialog
            className={`overflow-hidden rounded-[1.25rem] border border-border/90 bg-[color:var(--background)] text-[color:var(--foreground)] shadow-2xl shadow-zinc-950/25 outline-none ${dialogClassName ?? ''}`}
          >
            {children}
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal.Root>
  )
}

export const dialogHeaderCls =
  'flex items-start justify-between gap-4 border-b border-border/80 bg-surface/60 px-5 py-4 backdrop-blur'

export const dialogBodyCls = 'min-h-0 overflow-y-auto px-5 py-4'

export const dialogFooterCls =
  'flex items-center justify-end gap-2 border-t border-border/80 bg-surface/70 px-5 py-3 backdrop-blur'
