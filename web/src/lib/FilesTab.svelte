<script lang="ts">
	import type { FileEntry } from '$lib/api';

	let {
		uploadingFiles,
		onFilePickerChange,
		filesBreadcrumb,
		navigateToPath,
		navigateToBreadcrumb,
		filesError,
		onFilesDragOver,
		onFilesDragLeave,
		onFilesDrop,
		isDraggingOverFiles,
		loadingFiles,
		fileEntries,
		filesPath,
		navigateUp,
		renamingFile,
		renameInput = $bindable(''),
		onConfirmRename,
		onCancelRename,
		onOpenEntry,
		formatFileSize,
		onDownloadEntry,
		onStartRename,
		onDeleteEntry,
		editingFile,
		onCloseFileEditor,
		loadingFileContent,
		editingContent = $bindable(''),
		fileContentSaved = $bindable(false),
		fileContentError,
		savingFileContent,
		onSaveFileContent
	}: {
		uploadingFiles: boolean;
		onFilePickerChange: (e: Event) => void;
		filesBreadcrumb: () => string[];
		navigateToPath: (path: string) => void;
		navigateToBreadcrumb: (index: number) => void;
		filesError: string;
		onFilesDragOver: (e: DragEvent) => void;
		onFilesDragLeave: () => void;
		onFilesDrop: (e: DragEvent) => void;
		isDraggingOverFiles: boolean;
		loadingFiles: boolean;
		fileEntries: FileEntry[];
		filesPath: string;
		navigateUp: () => void;
		renamingFile: string | null;
		renameInput: string;
		onConfirmRename: () => void;
		onCancelRename: () => void;
		onOpenEntry: (entry: FileEntry) => void;
		formatFileSize: (bytes: number) => string;
		onDownloadEntry: (entry: FileEntry) => void;
		onStartRename: (entry: FileEntry) => void;
		onDeleteEntry: (entry: FileEntry) => void;
		editingFile: string | null;
		onCloseFileEditor: () => void;
		loadingFileContent: boolean;
		editingContent: string;
		fileContentSaved: boolean;
		fileContentError: string;
		savingFileContent: boolean;
		onSaveFileContent: () => void;
	} = $props();

	let pressedBackdrop = false;
</script>

<div class="border-border bg-card rounded-lg border p-4">
	<div class="flex items-center justify-between">
		<h2 class="font-medium">파일</h2>
		<label
			class="border-border cursor-pointer rounded-md border px-3 py-1.5 text-xs {uploadingFiles
				? 'opacity-50'
				: ''}"
		>
			{uploadingFiles ? '업로드 중...' : '업로드'}
			<input type="file" multiple class="hidden" disabled={uploadingFiles} onchange={onFilePickerChange} />
		</label>
	</div>

	<!-- Breadcrumb -->
	<div class="text-muted-foreground mt-2 flex flex-wrap items-center gap-1 text-xs">
		<button type="button" class="underline" onclick={() => navigateToPath('')}>루트</button>
		{#each filesBreadcrumb() as segment, i}
			<span>/</span>
			<button type="button" class="underline" onclick={() => navigateToBreadcrumb(i)}>{segment}</button>
		{/each}
	</div>

	{#if filesError}
		<p class="text-destructive mt-2 text-xs">{filesError}</p>
	{/if}

	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		ondragover={onFilesDragOver}
		ondragleave={onFilesDragLeave}
		ondrop={onFilesDrop}
		class="mt-2 rounded-md border {isDraggingOverFiles ? 'border-primary bg-primary/5' : 'border-border'}"
	>
		{#if loadingFiles}
			<p class="text-muted-foreground p-3 text-xs">불러오는 중...</p>
		{:else if fileEntries.length === 0}
			<p class="text-muted-foreground p-3 text-xs">
				빈 폴더입니다. 파일을 여기로 드래그해서 업로드할 수 있습니다.
			</p>
		{:else}
			<div class="divide-border divide-y">
				{#if filesPath}
					<!-- svelte-ignore a11y_click_events_have_key_events -->
					<!-- svelte-ignore a11y_no_static_element_interactions -->
					<div
						class="hover:bg-background/50 flex cursor-pointer items-center gap-2 px-3 py-2 text-sm"
						ondblclick={navigateUp}
						onclick={navigateUp}
					>
						<span>📁</span>
						<span class="text-muted-foreground">..</span>
					</div>
				{/if}
				{#each fileEntries as entry (entry.path)}
					{#if renamingFile === entry.path}
						<div class="flex items-center gap-2 px-3 py-2 text-sm">
							<span>{entry.is_dir ? '📁' : '📄'}</span>
							<input
								type="text"
								bind:value={renameInput}
								class="border-input bg-background min-w-0 flex-1 rounded-md border px-2 py-1 text-sm"
							/>
							<button
								class="bg-primary text-primary-foreground shrink-0 rounded-md px-2 py-1 text-xs"
								onclick={onConfirmRename}>저장</button
							>
							<button
								class="border-border shrink-0 rounded-md border px-2 py-1 text-xs"
								onclick={onCancelRename}>취소</button
							>
						</div>
					{:else}
						<!-- svelte-ignore a11y_click_events_have_key_events -->
						<!-- svelte-ignore a11y_no_static_element_interactions -->
						<div
							class="hover:bg-background/50 flex items-center gap-2 px-3 py-2 text-sm"
							ondblclick={() => onOpenEntry(entry)}
						>
							<span class="cursor-pointer" onclick={() => onOpenEntry(entry)}
								>{entry.is_dir ? '📁' : '📄'}</span
							>
							<span class="min-w-0 flex-1 cursor-pointer truncate" onclick={() => onOpenEntry(entry)}
								>{entry.name}</span
							>
							{#if !entry.is_dir}
								<span class="text-muted-foreground shrink-0 text-xs">{formatFileSize(entry.size)}</span>
							{/if}
							<div class="flex shrink-0 gap-1">
								<button
									class="border-border rounded-md border px-2 py-1 text-xs"
									onclick={() => onDownloadEntry(entry)}
									>{entry.is_dir ? '다운로드 (zip)' : '다운로드'}</button
								>
								<button
									class="border-border rounded-md border px-2 py-1 text-xs"
									onclick={() => onStartRename(entry)}>이름변경</button
								>
								<button
									class="border-border text-destructive rounded-md border px-2 py-1 text-xs"
									onclick={() => onDeleteEntry(entry)}>삭제</button
								>
							</div>
						</div>
					{/if}
				{/each}
			</div>
		{/if}
	</div>
</div>

{#if editingFile}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-8"
		onmousedown={(e) => (pressedBackdrop = e.target === e.currentTarget)}
		onclick={(e) => {
			if (pressedBackdrop && e.target === e.currentTarget) onCloseFileEditor();
		}}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="bg-card border-border flex max-h-[80vh] w-full max-w-2xl flex-col rounded-lg border p-4 shadow-lg"
		>
			<div class="mb-2 flex items-center justify-between">
				<h2 class="truncate font-medium">{editingFile}</h2>
				<button type="button" class="text-muted-foreground text-sm" onclick={onCloseFileEditor}
					>&times;</button
				>
			</div>
			{#if loadingFileContent}
				<p class="text-muted-foreground text-xs">불러오는 중...</p>
			{:else}
				<textarea
					bind:value={editingContent}
					oninput={() => (fileContentSaved = false)}
					rows="20"
					spellcheck="false"
					class="border-input bg-background w-full flex-1 rounded-md border p-2 font-mono text-xs"
				></textarea>
				{#if fileContentError}
					<p class="text-destructive mt-2 text-xs">{fileContentError}</p>
				{/if}
				<div class="mt-2 flex items-center gap-2">
					<button
						class="bg-primary text-primary-foreground rounded-md px-3 py-1.5 text-sm font-medium disabled:opacity-50"
						disabled={savingFileContent}
						onclick={onSaveFileContent}
					>
						{savingFileContent ? '저장 중...' : '저장'}
					</button>
					{#if fileContentSaved}
						<span class="text-muted-foreground text-xs">저장됨 · 재시작해야 반영됩니다</span>
					{/if}
				</div>
			{/if}
		</div>
	</div>
{/if}
