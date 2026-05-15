# Encrypted EFS file system holding the SQLite database file.
resource "aws_efs_file_system" "data" {
  creation_token = "${var.project_name}-data"
  encrypted      = true

  tags = {
    Name = "${var.project_name}-data"
  }
}

# One mount target per public subnet so the task can mount EFS from any AZ.
resource "aws_efs_mount_target" "data" {
  count = length(aws_subnet.public)

  file_system_id  = aws_efs_file_system.data.id
  subnet_id       = aws_subnet.public[count.index].id
  security_groups = [aws_security_group.efs.id]
}

# Access point pinned to the distroless "nonroot" uid/gid (65532) so the
# container can write /data without running as root.
resource "aws_efs_access_point" "data" {
  file_system_id = aws_efs_file_system.data.id

  posix_user {
    uid = 65532
    gid = 65532
  }

  root_directory {
    path = "/iou"

    creation_info {
      owner_uid   = 65532
      owner_gid   = 65532
      permissions = "0755"
    }
  }

  tags = {
    Name = "${var.project_name}-data-ap"
  }
}
