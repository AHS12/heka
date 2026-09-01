import type {ReactNode} from 'react'
import {Modal, useOverlayState} from '@heroui/react'
import type {ModalContainerProps} from '@heroui/react'

export function AppDialog({
  isOpen,
  onOpenChange,
  children,
  size = 'lg',
}: {
  isOpen: boolean
  onOpenChange: (isOpen: boolean) => void
  children: ReactNode
  size?: ModalContainerProps['size']
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
          <Modal.Dialog className="overflow-hidden rounded-[1.25rem] border border-zinc-200/90 bg-[color:var(--background)] text-[color:var(--foreground)] shadow-2xl shadow-zinc-950/25 outline-none dark:border-zinc-700/80">
            {children}
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal.Root>
  )
}

export const dialogHeaderCls =
  'flex items-start justify-between gap-4 border-b border-zinc-200/80 bg-white/60 px-5 py-4 backdrop-blur dark:border-zinc-800 dark:bg-zinc-950/35'

export const dialogBodyCls = 'min-h-0 overflow-y-auto px-5 py-4'

export const dialogFooterCls =
  'flex items-center justify-end gap-2 border-t border-zinc-200/80 bg-white/70 px-5 py-3 backdrop-blur dark:border-zinc-800 dark:bg-zinc-950/45'
