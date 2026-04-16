from app.models.context import AssembledContext


def build_answer_prompt(context: AssembledContext) -> str:
    blocks = []
    for block in context.blocks:
        location = block.file_path or block.chunk_id
        lines = ""
        if block.start_line is not None and block.end_line is not None:
            lines = f":{block.start_line}-{block.end_line}"
        blocks.append(f"[{location}{lines}]\n{block.content}")

    joined_blocks = "\n\n".join(blocks)
    return (
        "Answer the user's repository question using only the provided context. "
        "Cite relevant files and line numbers when available.\n\n"
        f"Question:\n{context.query}\n\n"
        f"Context:\n{joined_blocks}"
    )
