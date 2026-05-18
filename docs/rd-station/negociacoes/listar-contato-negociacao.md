Listar contatos da negociação

# Listar contatos da negociação

Listar contatos da negociação

# OpenAPI definition

```json
{
  "openapi": "3.1.1",
  "info": {
    "title": "RD Station CRM API v1",
    "version": "1.0",
    "description": "Documentação da API v1 do RD Station CRM.",
    "license": {
      "name": "Termos de Uso do Software RD Station CRM",
      "url": "https://legal.rdstation.com/pt/rdstation-crm-services-agreement/"
    },
    "termsOfService": "https://legal.rdstation.com/pt/rdstation-crm-services-agreement/",
    "contact": {
      "name": "Suporte RD Station",
      "url": "https://developers.rdstation.com/docs/suporte"
    }
  },
  "externalDocs": {
    "description": "Documentação completa",
    "url": "https://developers.rdstation.com/crm-v1"
  },
  "servers": [
    {
      "url": "https://crm.rdstation.com/api/v1",
      "description": "Production"
    }
  ],
  "security": [
    {
      "Token": []
    }
  ],
  "tags": [
    {
      "name": "crm-v1-deals",
      "description": "Negociações"
    }
  ],
  "paths": {
    "/deals/{deal_id}/contacts": {
      "get": {
        "summary": "Listar contatos da negociação",
        "operationId": "crm-v1-list-contacts-from-deal",
        "tags": [
          "crm-v1-deals"
        ],
        "description": "Listar contatos da negociação",
        "parameters": [
          {
            "name": "deal_id",
            "in": "path",
            "required": true,
            "description": "ID da negociação.",
            "schema": {
              "type": "string"
            }
          },
          {
            "name": "page",
            "in": "query",
            "schema": {
              "type": "string"
            },
            "description": "Página da listagem de contatos da negociação. Valor padrão é 1"
          },
          {
            "name": "limit",
            "in": "query",
            "schema": {
              "type": "string"
            },
            "description": "Limite de contatos que virão por listagem. Valor padrão é 20. Valor máximo é 200"
          }
        ],
        "requestBody": {
          "content": {
            "application/json": {
              "examples": {
                "Listar contatos da negociação": {
                  "value": ""
                }
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Success",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "contacts": {
                      "type": "array",
                      "items": {
                        "type": "object",
                        "properties": {
                          "_id": {
                            "type": "string",
                            "example": "ID"
                          },
                          "birthday": {
                            "type": [
                              "object",
                              "null"
                            ]
                          },
                          "contact_custom_fields": {
                            "type": "array",
                            "items": {}
                          },
                          "created_at": {
                            "type": "string",
                            "example": "2022-04-26T16:21:08.097-03:00"
                          },
                          "emails": {
                            "type": "array",
                            "items": {}
                          },
                          "facebook": {
                            "type": [
                              "string",
                              "null"
                            ]
                          },
                          "id": {
                            "type": "string",
                            "example": "ID"
                          },
                          "legal_bases": {
                            "type": "array",
                            "items": {
                              "type": "object",
                              "properties": {
                                "category": {
                                  "type": "string",
                                  "example": "communications"
                                },
                                "status": {
                                  "type": "string",
                                  "example": "granted"
                                },
                                "type": {
                                  "type": "string",
                                  "example": "consent"
                                }
                              }
                            }
                          },
                          "linkedin": {
                            "type": [
                              "string",
                              "null"
                            ]
                          },
                          "name": {
                            "type": "string",
                            "example": "NAME"
                          },
                          "notes": {
                            "type": [
                              "string",
                              "null"
                            ]
                          },
                          "organization_id": {
                            "type": "string",
                            "example": "ID"
                          },
                          "phones": {
                            "type": "array",
                            "items": {
                              "type": "object",
                              "properties": {
                                "created_at": {
                                  "type": [
                                    "string",
                                    "null"
                                  ],
                                  "format": "date-time"
                                },
                                "phone": {
                                  "type": "string",
                                  "example": "4888888"
                                },
                                "type": {
                                  "type": "string",
                                  "example": "work"
                                },
                                "updated_at": {
                                  "type": [
                                    "string",
                                    "null"
                                  ],
                                  "format": "date-time"
                                },
                                "whatsapp": {
                                  "type": "boolean",
                                  "example": true
                                },
                                "whatsapp_full_internacional": {
                                  "type": "string",
                                  "example": "+4888888"
                                },
                                "whatsapp_url_web": {
                                  "type": "string",
                                  "example": "https://web.whatsapp.com/send?phone=4888888&text"
                                }
                              }
                            }
                          },
                          "skype": {
                            "type": [
                              "string",
                              "null"
                            ]
                          },
                          "title": {
                            "type": "string",
                            "example": "Programador"
                          },
                          "updated_at": {
                            "type": "string",
                            "example": "2022-04-26T16:21:44.242-03:00"
                          }
                        }
                      }
                    },
                    "has_more": {
                      "type": "boolean",
                      "example": false
                    },
                    "total": {
                      "type": "number",
                      "example": 1
                    }
                  }
                },
                "examples": {
                  "Success": {
                    "value": {
                      "contacts": [
                        {
                          "_id": "ID",
                          "birthday": null,
                          "contact_custom_fields": [],
                          "created_at": "2022-04-26T16:21:08.097-03:00",
                          "emails": [],
                          "facebook": null,
                          "id": "ID",
                          "legal_bases": [
                            {
                              "category": "communications",
                              "status": "granted",
                              "type": "consent"
                            }
                          ],
                          "linkedin": null,
                          "name": "NAME",
                          "notes": null,
                          "organization_id": "ID",
                          "phones": [
                            {
                              "created_at": null,
                              "phone": "4888888",
                              "type": "work",
                              "updated_at": null,
                              "whatsapp": true,
                              "whatsapp_full_internacional": "+4888888",
                              "whatsapp_url_web": "https://web.whatsapp.com/send?phone=4888888&text"
                            }
                          ],
                          "skype": null,
                          "title": "Programador",
                          "updated_at": "2022-04-26T16:21:44.242-03:00"
                        }
                      ],
                      "has_more": false,
                      "total": 1
                    }
                  }
                }
              }
            }
          }
        }
      }
    }
  },
  "components": {
    "securitySchemes": {
      "Token": {
        "type": "apiKey",
        "name": "token",
        "in": "query",
        "description": "Token do usuário"
      }
    }
  }
}
```