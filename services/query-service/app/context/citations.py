from app.models.answer import Citation
from app.models.context import ContextBlock


def citations_from_context(blocks: list[ContextBlock]) -> list[Citation]:
    return [
        Citation(
            chunk_id=block.chunk_id,
            file_path=block.file_path,
            start_line=block.start_line,
            end_line=block.end_line,
        )
        for block in blocks
    ]
