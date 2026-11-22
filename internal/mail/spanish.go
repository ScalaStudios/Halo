package mail

import "halo/internal/store"

const saludo = "Hola{{if .Name}}, {{.Name}}{{end}}:\n\n"

var translations = map[string]map[string]store.EmailTemplate{"es": {
	"invite": {Subject: "Configura tu cuenta de Halo", Body: saludo +
		"{{.Administrator}} te invitó a Halo, donde inicias sesión en tus aplicaciones de trabajo con una clave de acceso en lugar de una contraseña.\n\n" +
		"Configura tu clave de acceso con este enlace:\n\n{{.Link}}\n\n" +
		"El enlace funciona una vez y caduca el {{.Expires}}. Si no esperabas esta invitación, puedes ignorar este correo."},
	"reset": {Subject: "Restablece cómo inicias sesión en Halo", Body: saludo +
		"{{.Administrator}} restableció cómo inicias sesión en Halo. Tus claves de acceso, llaves de seguridad, aplicaciones de autenticación y códigos de recuperación ya no funcionan, y se cerraron todas tus sesiones.\n\n" +
		"Configura una nueva clave de acceso con este enlace:\n\n{{.Link}}\n\n" +
		"El enlace funciona una vez y caduca el {{.Expires}}. Si no esperabas este cambio, ponte en contacto con tu administrador antes de usarlo."},
	"magic_link": {Subject: "Tu enlace de inicio de sesión de Halo", Body: saludo +
		"Abre este enlace para iniciar sesión en {{if .Application}}{{.Application}}{{else}}Halo{{end}}:\n\n{{.Link}}\n\n" +
		"Funciona una vez y caduca en 10 minutos.{{if .Application}} Ábrelo en el navegador donde empezaste a iniciar sesión.{{end}} Si no pediste iniciar sesión, puedes ignorar este correo."},
	"magic_code": {Subject: "Tu código de inicio de sesión de Halo", Body: saludo +
		"Introduce este código en la página de inicio de sesión para entrar en {{if .Application}}{{.Application}}{{else}}Halo{{end}}:\n\n{{.Code}}\n\n" +
		"{{if .Link}}O abre este enlace{{if .Application}} en el navegador donde empezaste a iniciar sesión{{end}}:\n\n{{.Link}}\n\nCada uno funciona{{else}}Funciona{{end}}" +
		" una vez y caduca en 10 minutos. No compartas el código con nadie. Si no pediste iniciar sesión, puedes ignorar este correo."},
	"sign_up": {Subject: "Termina de registrarte en Halo", Body: saludo +
		"Confirma tu dirección de correo y crea tu clave de acceso para terminar de registrarte en Halo:\n\n{{.Link}}\n\n" +
		"El enlace funciona una vez y caduca el {{.Expires}}. Si no te registraste, puedes ignorar este correo y no se creará ninguna cuenta."},
	"access_request": {Subject: "{{.Requester}} solicita {{.Package}}", Body: "{{if .RequestedBy}}{{.RequestedBy}} solicitó el paquete de acceso {{.Package}} para {{.Requester}} ({{.RequesterEmail}}) {{.Duration}}." +
		"{{else}}{{.Requester}} ({{.RequesterEmail}}) solicitó el paquete de acceso {{.Package}} {{.Duration}}.{{end}}" +
		"{{if .Reason}}\n\nMotivo: {{.Reason}}{{end}}{{if .Answers}}\n\n{{.Answers}}{{end}}\n\nAprueba o rechaza la solicitud en Halo: {{.Link}}"},
	"access_request_for_you": {Subject: "{{.RequestedBy}} solicitó {{.Package}} para ti", Body: "{{.RequestedBy}} solicitó el paquete de acceso {{.Package}} para ti {{.Duration}}. " +
		"Recibirás otro correo cuando se tome una decisión.\n\nSi no lo necesitas, cancela la solicitud: {{.Link}}"},
	"access_expired": {Subject: "Tu solicitud de {{.Package}} caducó", Body: "Nadie decidió sobre tu solicitud de {{.Package}} antes del plazo, así que Halo la cerró.\n\n" +
		"Si todavía necesitas acceso, vuelve a solicitarlo: {{.Link}}"},
	"access_approved": {Subject: "Se aprobó tu solicitud de {{.Package}}", Body: "{{.Approver}} aprobó tu solicitud de {{.Package}}. Tienes acceso {{if .Expires}}hasta el {{.Expires}}{{else}}hasta que se revoque{{end}}.\n\n" +
		"Grupos: {{.Groups}}. Las aplicaciones aplican el cambio la próxima vez que inicias sesión en ellas.{{if .Note}}\n\nNota de {{.Approver}}: {{.Note}}{{end}}\n\nConsulta tus solicitudes: {{.Link}}"},
	"access_denied": {Subject: "Se rechazó tu solicitud de {{.Package}}", Body: "{{.Approver}} rechazó tu solicitud de {{.Package}}." +
		"{{if .Note}}\n\nNota de {{.Approver}}: {{.Note}}{{end}}\n\nConsulta tus solicitudes: {{.Link}}"},
	"access_pass": {Subject: "Se creó un pase de acceso temporal para ti", Body: saludo +
		"{{.Administrator}} creó un pase de acceso temporal para tu cuenta de Halo. Te permite configurar una nueva clave de acceso y deja de funcionar el {{.Expires}}.\n\n" +
		"Halo nunca envía el pase por correo, así que {{.Administrator}} te lo dará en persona o por teléfono. Si no lo pediste, ponte en contacto con tu administrador."},
	"new_sign_in": {Subject: "Nuevo inicio de sesión en tu cuenta de Halo", Body: saludo +
		"Se inició sesión en tu cuenta de Halo{{if .Application}} para entrar en {{.Application}}{{end}} desde un dispositivo o una red que no había usado antes.\n\nCuándo: {{.Time}}\nDispositivo: {{.Device}}\nDirección IP: {{.IP}}\n\n" +
		"Si fuiste tú, no tienes que hacer nada. Si no, cierra la sesión de ese dispositivo en el portal de tu cuenta, reemplaza tus claves de acceso y avisa a tu administrador:\n\n{{.Link}}"},
	"method_added": {Subject: "Se añadió un método de inicio de sesión a tu cuenta de Halo", Body: saludo +
		"{{.Method}} se añadió a tu cuenta de Halo el {{.Time}} y ya se puede usar para iniciar sesión como tú.\n\n" +
		"Si no fuiste tú, elimina ese método en el portal de tu cuenta y avisa a tu administrador:\n\n{{.Link}}"},
	"method_removed": {Subject: "Se eliminó un método de inicio de sesión de tu cuenta de Halo", Body: saludo +
		"{{if .Administrator}}{{.Administrator}} eliminó {{.Method}} de tu cuenta de Halo{{else}}{{.Method}} se eliminó de tu cuenta de Halo{{end}} el {{.Time}} y ya no sirve para iniciar sesión.\n\n" +
		"Si no esperabas este cambio, revisa tus métodos de inicio de sesión en el portal de tu cuenta y avisa a tu administrador:\n\n{{.Link}}"},
	"recovery_codes": {Subject: "Nuevos códigos de recuperación para tu cuenta de Halo", Body: saludo +
		"Se generaron nuevos códigos de recuperación para tu cuenta de Halo el {{.Time}}, y los anteriores ya no funcionan.\n\n" +
		"Si no fuiste tú, genera códigos nuevos en el portal de tu cuenta y avisa a tu administrador:\n\n{{.Link}}"},
	"email_changed": {Subject: "Cambió la dirección de correo de tu cuenta de Halo", Body: saludo +
		"{{.Administrator}} cambió la dirección de correo de tu cuenta de Halo de {{.OldEmail}} a {{.NewEmail}} el {{.Time}}. Halo envió este correo a ambas direcciones.\n\n" +
		"Si no esperabas este cambio, ponte en contacto con tu administrador. Revisa tu cuenta en el portal:\n\n{{.Link}}"},
	"account_locked": {Subject: "Tu cuenta de Halo está bloqueada", Body: saludo +
		"Halo bloqueó tu cuenta tras {{.Attempts}} contraseñas o códigos incorrectos. Puedes volver a intentarlo después del {{.Until}}.\n\n" +
		"Si esos intentos no fueron tuyos, es posible que alguien esté intentando adivinar tu contraseña. Revisa tus inicios de sesión recientes en el portal de tu cuenta y avisa a tu administrador:\n\n{{.Link}}"},
	"account_locked_admin": {Subject: "La cuenta de Halo de {{.Person}} está bloqueada", Body: "Halo bloqueó la cuenta de {{.Person}} ({{.PersonEmail}}) tras {{.Attempts}} contraseñas o códigos incorrectos. Podrá volver a intentarlo después del {{.Until}}.\n\n" +
		"Consulta los datos de la persona y sus inicios de sesión recientes en Halo: {{.Link}}"},
	"impersonation": {Subject: "{{.Administrator}} está viendo Halo como tú", Body: saludo +
		"{{.Administrator}} empezó a ver tu cuenta de Halo como si fuera tú, para ayudarte con: {{.Purpose}}\n\n" +
		"Puede consultar la información, pero no cambiar nada, y su acceso termina a más tardar el {{.Until}}. Si no esperabas esto, avisa a tu equipo de TI o termínalo ahora cerrando tus otras sesiones:\n\n{{.Link}}"},
	"risk_response": {Subject: "Inicio de sesión de riesgo en tu cuenta de Halo", Body: saludo +
		"Halo detectó un inicio de sesión de riesgo en tu cuenta el {{.Time}}: {{.Risk}}\n\n{{if .Response}}{{.Response}}\n\n{{end}}" +
		"Si fuiste tú, no tienes que hacer nada. Si no, reemplaza tus claves de acceso, revisa tus inicios de sesión recientes y avisa a tu administrador:\n\n{{.Link}}"},
	"guest_expiring": {Subject: "El acceso de invitado de {{.Guest}} termina pronto", Body: "La cuenta de invitado de {{.Guest}} ({{.GuestEmail}}) deja de funcionar el {{.Expires}}, y Halo la suspenderá en ese momento.\n\n" +
		"Si todavía necesita acceso, amplíalo en Halo: {{.Link}}\n\nSi no, no tienes que hacer nada."},
}}
