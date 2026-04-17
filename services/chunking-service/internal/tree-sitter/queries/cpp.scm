(function_definition
  declarator: (function_declarator
    declarator: (identifier) @function.name)) @function.definition

(function_definition
  declarator: (function_declarator
    declarator: (qualified_identifier
      name: (_) @method.name))) @method.definition

(function_definition
  declarator: (function_declarator
    declarator: (field_identifier) @method.name)) @method.definition

(class_specifier
  name: (type_identifier) @class.name) @class.definition

(struct_specifier
  name: (type_identifier) @class.name) @class.definition

(union_specifier
  name: (type_identifier) @class.name) @class.definition

(namespace_definition
  name: [(namespace_identifier) (identifier)] @module.name) @module.declaration

(preproc_include
  path: [(string_literal) (system_lib_string)] @import.path) @import.definition

(comment) @doc.comment
